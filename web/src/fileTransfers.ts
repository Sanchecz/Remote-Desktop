export type FileTransferProgress = {
  received: number;
  total: number;
  percent: number;
  bytesPerSecond: number;
  remainingSeconds: number | null;
};

export function fileTransferProgress(received: number, total: number, startedAt: number, now = Date.now()): FileTransferProgress {
  const safeTotal = Math.max(0, Number.isFinite(total) ? total : 0);
  const safeReceived = Math.min(safeTotal || Number.MAX_SAFE_INTEGER, Math.max(0, Number.isFinite(received) ? received : 0));
  const elapsedSeconds = Math.max(0.25, (Math.max(now, startedAt) - startedAt) / 1_000);
  const bytesPerSecond = safeReceived / elapsedSeconds;
  const remaining = safeTotal > safeReceived && bytesPerSecond > 0 ? (safeTotal - safeReceived) / bytesPerSecond : null;
  return {
    received: safeReceived,
    total: safeTotal,
    percent: safeTotal > 0 ? Math.min(100, Math.max(0, safeReceived * 100 / safeTotal)) : 100,
    bytesPerSecond,
    remainingSeconds: remaining != null && Number.isFinite(remaining) ? remaining : null
  };
}

export function abortableDelay(milliseconds: number, signal?: AbortSignal) {
  if (signal?.aborted) return Promise.reject(new DOMException("Передача отменена", "AbortError"));
  return new Promise<void>((resolve, reject) => {
    const timer = globalThis.setTimeout(done, Math.max(0, milliseconds));
    function done() {
      signal?.removeEventListener("abort", aborted);
      resolve();
    }
    function aborted() {
      globalThis.clearTimeout(timer);
      signal?.removeEventListener("abort", aborted);
      reject(new DOMException("Передача отменена", "AbortError"));
    }
    signal?.addEventListener("abort", aborted, { once: true });
  });
}

export function isAbortError(reason: unknown) {
  return reason instanceof DOMException && reason.name === "AbortError";
}

export type UploadedTransferChunk = {
  received: number;
  size: number;
};

export function validateTransferCheckpoint(
  progress: UploadedTransferChunk,
  offset: number,
  chunkSize: number,
  totalSize: number
): UploadedTransferChunk {
  const expected = offset + chunkSize;
  if (
    !Number.isSafeInteger(progress.received) ||
    !Number.isSafeInteger(progress.size) ||
    !Number.isSafeInteger(offset) ||
    !Number.isSafeInteger(chunkSize) ||
    !Number.isSafeInteger(totalSize) ||
    offset < 0 ||
    chunkSize < 0 ||
    totalSize < 0 ||
    progress.size !== totalSize ||
    expected > totalSize ||
    progress.received !== expected
  ) {
    throw new Error("Некорректная контрольная точка передачи");
  }
  return progress;
}

function transferError(xhr: XMLHttpRequest): Error {
  try {
    const payload = JSON.parse(xhr.responseText || "{}") as { error?: string };
    if (payload.error) return new Error(payload.error);
  } catch {
    // A proxy can return plain text or HTML. The HTTP status below is still a
    // useful, safe error and the response body is not exposed to the user.
  }
  return new Error(xhr.status ? `HTTP ${xhr.status}` : "Соединение прервано");
}

/**
 * Streams one resumable browser -> server chunk while exposing byte progress.
 * XMLHttpRequest is intentional: Fetch upload progress is not interoperable
 * across Chrome, Edge, Firefox and Safari yet.
 */
export function uploadTransferChunk(
  url: string,
  body: Blob,
  csrf: string,
  signal: AbortSignal,
  onProgress: (sent: number) => void
): Promise<UploadedTransferChunk> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException("Передача отменена", "AbortError"));
      return;
    }

    const xhr = new XMLHttpRequest();
    let settled = false;
    const finish = (callback: () => void) => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", abort);
      callback();
    };
    const abort = () => {
      xhr.abort();
      finish(() => reject(new DOMException("Передача отменена", "AbortError")));
    };

    xhr.open("PUT", url, true);
    xhr.withCredentials = true;
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.setRequestHeader("X-CSRF-Token", csrf);
    xhr.upload.onprogress = (event) => onProgress(Math.min(body.size, Math.max(0, event.loaded)));
    xhr.onerror = () => finish(() => reject(transferError(xhr)));
    xhr.onabort = () => finish(() => reject(new DOMException("Передача отменена", "AbortError")));
    xhr.onload = () => finish(() => {
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(transferError(xhr));
        return;
      }
      try {
        const progress = JSON.parse(xhr.responseText) as UploadedTransferChunk;
        if (!Number.isFinite(progress.received) || !Number.isFinite(progress.size)) throw new Error("Некорректный ответ сервера");
        onProgress(body.size);
        resolve(progress);
      } catch (reason) {
        reject(reason instanceof Error ? reason : new Error("Некорректный ответ сервера"));
      }
    });
    signal.addEventListener("abort", abort, { once: true });
    xhr.send(body);
  });
}
