export type RemoteClipboardPayload =
	| { kind: "text"; text: string; order: number }
	| { kind: "image"; image: Blob; order: number };

export type RemoteClipboardCopyGate = {
	afterAckID: number;
	baselineSequence: number;
	baselineText: string | null;
	requestedAt: number;
};

export type RemoteClipboardSyncState = "ready" | "pending" | "syncing" | "error";

export function remoteClipboardActionLabel(state: RemoteClipboardSyncState, compact = false): string {
	if (state === "pending") return compact ? "Получить буфер" : "Получить с удалённого ПК";
	if (state === "syncing") return "Синхронизация…";
	if (state === "error") return compact ? "Повторить" : "Повторить синхронизацию";
	return compact ? "Синхронизировать" : "Синхронизировать буфер";
}

// Copy can reach the Agent after the first read request on a congested VDI
// session. Sampling a few bounded times avoids resolving WebKit's deferred
// ClipboardItem with the value which existed before Ctrl/Cmd+C.
export const REMOTE_CLIPBOARD_COPY_READ_DELAYS = [140, 320, 650, 1100] as const;
export const REMOTE_CLIPBOARD_COPY_TIMEOUT = 4_000;

export function newerRemoteClipboardPayload(
	current: RemoteClipboardPayload | null,
	incoming: RemoteClipboardPayload,
): RemoteClipboardPayload {
	return !current || incoming.order >= current.order ? incoming : current;
}

export function sameRemoteClipboardPayload(left: RemoteClipboardPayload | null, right: RemoteClipboardPayload | null): boolean {
	if (!left || !right || left.kind !== right.kind || left.order !== right.order) return false;
	return left.kind === "text"
		? left.text === (right as Extract<RemoteClipboardPayload, { kind: "text" }>).text
		: left.image === (right as Extract<RemoteClipboardPayload, { kind: "image" }>).image;
}

export function shouldResolveRemoteClipboardCopy(
	gate: RemoteClipboardCopyGate,
	acknowledgement: { id: number; sequence?: number; text: string },
	now: number,
): boolean {
	if (acknowledgement.id <= gate.afterAckID) return false;
	const sequence = Math.max(0, Number(acknowledgement.sequence) || 0);
	if (gate.baselineSequence > 0 && sequence > 0) return sequence !== gate.baselineSequence;
	// Old Agents did not report GetClipboardSequenceNumber. Preserve compatibility
	// while refusing an obviously stale first read for a bounded settle window.
	if (acknowledgement.text !== gate.baselineText) return true;
	return now - gate.requestedAt >= 650;
}

export function textClipboardFingerprint(text: string): string {
	return `text:${text}`;
}

export function imageClipboardFingerprint(hash: string): string {
	return `image:${hash}`;
}
