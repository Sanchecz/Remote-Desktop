export type PointerCadenceResult<T> = {
	send: T | null;
	delayMs: number;
};

// Pointer events can arrive much faster than the remote desktop can consume
// them (240 Hz touch sampling is common on current phones). Keep the leading
// sample for latency and one newest trailing sample for accuracy. Unlike a
// simple throttle, the final coordinate is therefore never stranded locally
// when the finger stops between two cadence boundaries.
export class LatestPointerCadence<T> {
	private lastSentAt = Number.NEGATIVE_INFINITY;
	private pending: T | null = null;
	private intervalMs: number;

	constructor(intervalMs: number) {
		this.intervalMs = intervalMs;
	}

	setInterval(intervalMs: number) {
		this.intervalMs = Math.max(1, Number.isFinite(intervalMs) ? intervalMs : this.intervalMs);
	}

	offer(value: T, now: number): PointerCadenceResult<T> {
		const timestamp = Number.isFinite(now) ? now : 0;
		this.pending = value;
		const delayMs = this.remaining(timestamp);
		if (delayMs > 0) return { send: null, delayMs };
		return { send: this.take(timestamp, true), delayMs: 0 };
	}

	take(now: number, force = false): T | null {
		if (this.pending === null) return null;
		const timestamp = Number.isFinite(now) ? now : 0;
		if (!force && this.remaining(timestamp) > 0) return null;
		const value = this.pending;
		this.pending = null;
		this.lastSentAt = Math.max(this.lastSentAt, timestamp);
		return value;
	}

	remaining(now: number): number {
		if (this.pending === null) return 0;
		const timestamp = Number.isFinite(now) ? now : 0;
		return Math.max(0, Math.max(1, this.intervalMs) - (timestamp - this.lastSentAt));
	}

	clear() {
		this.pending = null;
		this.lastSentAt = Number.NEGATIVE_INFINITY;
	}
}

export function remotePointerCadenceMillis(coarsePointer: boolean, bufferedBytes = 0): number {
	const buffered = Number.isFinite(bufferedBytes) ? Math.max(0, bufferedBytes) : 0;
	// Pointer packets have their own low-bandwidth WebSocket, separate from all
	// JPEG lanes. At a healthy queue, preserve 120 Hz phone sampling and a fast
	// desktop mouse instead of needlessly tying input to a video-frame interval.
	// Back off only when the browser reports an actual pending-byte buildup; the
	// latest-only cadence then prevents a stale trail of coordinates.
	if (buffered >= 64 * 1024) return 20;
	if (buffered >= 16 * 1024) return 12;
	return coarsePointer ? 8 : 6;
}
