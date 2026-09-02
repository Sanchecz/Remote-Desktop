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

export const REMOTE_CLIPBOARD_DIRECTION_TITLE = {
	send: "С этого устройства → удалённый ПК",
	receive: "С удалённого ПК → это устройство",
} as const;

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

// GetClipboardSequenceNumber is an unsigned 32-bit counter and eventually
// wraps. Half-range comparison accepts equal/current values for manual reads,
// accepts a genuine wrap and rejects a delayed acknowledgement for an older
// clipboard snapshot.
export function isRemoteClipboardSequenceNewer(current: number, incoming: number): boolean {
	const left = Math.max(0, Number(current) || 0) >>> 0;
	const right = Math.max(0, Number(incoming) || 0) >>> 0;
	if (right === 0) return false;
	if (left === 0) return true;
	const delta = (right - left) >>> 0;
	return delta > 0 && delta < 0x80000000;
}

export function shouldAcceptRemoteClipboardSequence(current: number, incoming: number): boolean {
	const right = Math.max(0, Number(incoming) || 0) >>> 0;
	if (right === 0) return true; // Compatibility with Agents before sequence reporting.
	const left = Math.max(0, Number(current) || 0) >>> 0;
	return left === 0 || right === left || isRemoteClipboardSequenceNewer(left, right);
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
	if (gate.baselineSequence > 0 && sequence > 0) return isRemoteClipboardSequenceNewer(gate.baselineSequence, sequence);
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
