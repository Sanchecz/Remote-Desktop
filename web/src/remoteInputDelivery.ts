export type RemoteInputEvent = Record<string, unknown>;

export type PendingRemoteInputBatch<TSocket = unknown> = {
	events: RemoteInputEvent[];
	socket: TSocket;
	expiresAt: number;
};

export function remoteInputClientID(prefix: string, sequence: number): string {
	const safePrefix = prefix.replace(/[^A-Za-z0-9_.:-]/g, "_").slice(0, 72) || "viewer";
	return `${safePrefix}:${Math.max(1, Math.trunc(sequence))}`;
}

export function remoteInputBatchID(prefix: string, sequence: number): string {
	const safePrefix = prefix.replace(/[^A-Za-z0-9_.:-]/g, "_").slice(0, 72) || "viewer";
	return `${safePrefix}:b${Math.max(1, Math.trunc(sequence))}`;
}

export function bindRemoteInputCoordinates(
	event: RemoteInputEvent,
	coordinateWidth: number,
	coordinateHeight: number,
): RemoteInputEvent {
	return { ...event, coordinateWidth, coordinateHeight };
}

export function remoteInputAckID(payload: unknown): string {
	if (typeof payload !== "string" || payload.length > 512) return "";
	try {
		const parsed = JSON.parse(payload) as { inputAck?: unknown };
		return typeof parsed.inputAck === "string" ? parsed.inputAck : "";
	} catch {
		return "";
	}
}

// A cancelled HTTP fallback may reject only after React has already disposed
// the old viewer and mounted another device. Never put that stale batch into
// the shared input queue of the new session.
export function shouldRetryRemoteInputDelivery(activeSessionID: string, attemptedSessionID: string): boolean {
	return activeSessionID !== "" && activeSessionID === attemptedSessionID;
}

export function takePendingRemoteInputBatches<TSocket>(
	pending: Map<string, PendingRemoteInputBatch<TSocket>>,
	shouldTake: (batch: PendingRemoteInputBatch<TSocket>) => boolean,
): RemoteInputEvent[] {
	const retry: RemoteInputEvent[] = [];
	for (const [batchID, batch] of pending) {
		if (!shouldTake(batch)) continue;
		pending.delete(batchID);
		retry.push(...batch.events);
	}
	return retry;
}

function isFreePointerMove(event: RemoteInputEvent): boolean {
	return event.type === "pointer" && event.action === "move";
}

// An HTTP fallback keeps a batch locally until the server acknowledges it. If
// the response is lost after the server accepted the request, clientInputId
// makes the retry idempotent. Only adjacent free pointer moves are collapsed;
// clicks, key/button releases, wheel packets and text always retain order.
export function restoreRemoteInputBatch(retry: RemoteInputEvent[], queued: RemoteInputEvent[]): RemoteInputEvent[] {
	const restored: RemoteInputEvent[] = [];
	for (const event of [...retry, ...queued]) {
		if (isFreePointerMove(event) && isFreePointerMove(restored.at(-1) || {})) {
			restored[restored.length - 1] = event;
			continue;
		}
		restored.push(event);
	}
	return restored;
}
