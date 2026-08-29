import assert from "node:assert/strict";
import { afterEach, describe, it } from "node:test";
import { abortableDelay, fileTransferProgress, isAbortError, uploadTransferChunk, validateTransferCheckpoint } from "../src/fileTransfers.ts";

const nativeXMLHttpRequest = globalThis.XMLHttpRequest;

class FakeXMLHttpRequest {
  static instances: FakeXMLHttpRequest[] = [];

  upload: { onprogress: ((event: { loaded: number }) => void) | null } = { onprogress: null };
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  onload: (() => void) | null = null;
  status = 0;
  responseText = "";
  withCredentials = false;
  method = "";
  url = "";
  headers = new Map<string, string>();
  sentBody: Blob | null = null;
  aborted = false;

  constructor() { FakeXMLHttpRequest.instances.push(this); }
  open(method: string, url: string) { this.method = method; this.url = url; }
  setRequestHeader(name: string, value: string) { this.headers.set(name, value); }
  send(body: Blob) { this.sentBody = body; }
  abort() { this.aborted = true; this.onabort?.(); }
}

function installFakeXMLHttpRequest() {
  FakeXMLHttpRequest.instances = [];
  globalThis.XMLHttpRequest = FakeXMLHttpRequest as unknown as typeof XMLHttpRequest;
}

afterEach(() => {
  globalThis.XMLHttpRequest = nativeXMLHttpRequest;
  FakeXMLHttpRequest.instances = [];
});

describe("file transfer telemetry", () => {
  it("computes bounded progress, average speed and ETA", () => {
    const progress = fileTransferProgress(50 * 1024 * 1024, 200 * 1024 * 1024, 1_000, 6_000);
    assert.equal(progress.percent, 25);
    assert.equal(progress.bytesPerSecond, 10 * 1024 * 1024);
    assert.equal(progress.remainingSeconds, 15);
  });

  it("clamps stale or invalid byte counters", () => {
    assert.equal(fileTransferProgress(-1, 100, 0, 1_000).received, 0);
    assert.equal(fileTransferProgress(150, 100, 0, 1_000).percent, 100);
    assert.equal(fileTransferProgress(Number.NaN, Number.NaN, 0, 1_000).percent, 100);
  });
});

describe("abortable transfer waits", () => {
  it("stops immediately when the transfer is cancelled", async () => {
    const controller = new AbortController();
    const pending = abortableDelay(10_000, controller.signal);
    controller.abort();
    await assert.rejects(pending, isAbortError);
  });
});

describe("resumable browser uploads", () => {
	it("accepts only the exact committed chunk boundary", () => {
		assert.deepEqual(validateTransferCheckpoint({ received: 68, size: 100 }, 4, 64, 100), { received: 68, size: 100 });
		assert.throws(() => validateTransferCheckpoint({ received: 67, size: 100 }, 4, 64, 100), /контрольная точка/i);
		assert.throws(() => validateTransferCheckpoint({ received: 101, size: 100 }, 68, 32, 100), /контрольная точка/i);
		assert.throws(() => validateTransferCheckpoint({ received: 68, size: 99 }, 4, 64, 100), /контрольная точка/i);
	});

  it("reports progress within a chunk and validates the checkpoint", async () => {
    installFakeXMLHttpRequest();
    const controller = new AbortController();
    const progress: number[] = [];
    const body = new Blob([new Uint8Array([1, 2, 3, 4])]);
    const pending = uploadTransferChunk("/api/file-transfers/id/data?offset=0", body, "csrf", controller.signal, (sent) => progress.push(sent));
    const xhr = FakeXMLHttpRequest.instances[0];

    assert.equal(xhr.method, "PUT");
    assert.equal(xhr.url, "/api/file-transfers/id/data?offset=0");
    assert.equal(xhr.withCredentials, true);
    assert.equal(xhr.headers.get("Content-Type"), "application/octet-stream");
    assert.equal(xhr.headers.get("X-CSRF-Token"), "csrf");
    assert.equal(xhr.sentBody, body);

    xhr.upload.onprogress?.({ loaded: 2 });
    xhr.status = 200;
    xhr.responseText = JSON.stringify({ received: 4, size: 4 });
    xhr.onload?.();

    assert.deepEqual(await pending, { received: 4, size: 4 });
    assert.deepEqual(progress, [2, 4]);
  });

  it("aborts the network request and rejects with AbortError", async () => {
    installFakeXMLHttpRequest();
    const controller = new AbortController();
    const pending = uploadTransferChunk("/upload", new Blob(["payload"]), "csrf", controller.signal, () => undefined);
    const xhr = FakeXMLHttpRequest.instances[0];

    controller.abort();

    await assert.rejects(pending, isAbortError);
    assert.equal(xhr.aborted, true);
  });

  it("surfaces safe server errors and rejects malformed checkpoints", async () => {
    installFakeXMLHttpRequest();
    const controller = new AbortController();
    const failed = uploadTransferChunk("/upload", new Blob(["a"]), "csrf", controller.signal, () => undefined);
    const failedXHR = FakeXMLHttpRequest.instances[0];
    failedXHR.status = 409;
    failedXHR.responseText = JSON.stringify({ error: "Смещение части файла устарело" });
    failedXHR.onload?.();
    await assert.rejects(failed, /Смещение части файла устарело/);

    const malformed = uploadTransferChunk("/upload", new Blob(["b"]), "csrf", controller.signal, () => undefined);
    const malformedXHR = FakeXMLHttpRequest.instances[1];
    malformedXHR.status = 200;
    malformedXHR.responseText = JSON.stringify({ received: "wrong", size: 1 });
    malformedXHR.onload?.();
    await assert.rejects(malformed, /Некорректный ответ сервера/);
  });
});
