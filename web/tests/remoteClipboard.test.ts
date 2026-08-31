import assert from "node:assert/strict";
import test from "node:test";
import { imageClipboardFingerprint, newerRemoteClipboardPayload, REMOTE_CLIPBOARD_COPY_READ_DELAYS, remoteClipboardActionLabel, shouldResolveRemoteClipboardCopy, textClipboardFingerprint } from "../src/remoteClipboard.ts";

test("latest remote clipboard payload wins without text/image races", () => {
	const image = new Blob([new Uint8Array([1, 2, 3])], { type: "image/png" });
	const text = { kind: "text" as const, text: "old", order: 3 };
	const newerImage = { kind: "image" as const, image, order: 4 };
	assert.equal(newerRemoteClipboardPayload(text, newerImage), newerImage);
	assert.equal(newerRemoteClipboardPayload(newerImage, text), newerImage);
});

test("deferred remote copy waits for the Windows clipboard sequence to change", () => {
	const gate = { afterAckID: 8, baselineSequence: 41, baselineText: "before", requestedAt: 1000 };
	assert.equal(shouldResolveRemoteClipboardCopy(gate, { id: 8, sequence: 42, text: "after" }, 1100), false);
	assert.equal(shouldResolveRemoteClipboardCopy(gate, { id: 9, sequence: 41, text: "before" }, 1200), false);
	assert.equal(shouldResolveRemoteClipboardCopy(gate, { id: 10, sequence: 42, text: "after" }, 1250), true);
});

test("old Agents use content change and a bounded same-content fallback", () => {
	const gate = { afterAckID: 3, baselineSequence: 0, baselineText: "same", requestedAt: 1000 };
	assert.equal(shouldResolveRemoteClipboardCopy(gate, { id: 4, text: "same" }, 1300), false);
	assert.equal(shouldResolveRemoteClipboardCopy(gate, { id: 5, text: "changed" }, 1300), true);
	assert.equal(shouldResolveRemoteClipboardCopy(gate, { id: 6, text: "same" }, 1700), true);
	assert.deepEqual(REMOTE_CLIPBOARD_COPY_READ_DELAYS, [140, 320, 650, 1100]);
});

test("text and image fingerprints cannot collide", () => {
	assert.equal(textClipboardFingerprint("abc"), "text:abc");
	assert.equal(imageClipboardFingerprint("abc"), "image:abc");
	assert.notEqual(textClipboardFingerprint("abc"), imageClipboardFingerprint("abc"));
});

test("clipboard action explains what the button will do", () => {
	assert.equal(remoteClipboardActionLabel("ready"), "Синхронизировать буфер");
	assert.equal(remoteClipboardActionLabel("pending"), "Получить с удалённого ПК");
	assert.equal(remoteClipboardActionLabel("syncing"), "Синхронизация…");
	assert.equal(remoteClipboardActionLabel("error", true), "Повторить");
});
