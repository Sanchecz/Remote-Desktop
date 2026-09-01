export const REMOTE_RECONNECT_DELAYS = [250, 500, 1_000, 2_000, 4_000, 8_000] as const;

export function remoteReconnectDelay(attempt: number) {
	if (!Number.isFinite(attempt) || attempt <= 0) return REMOTE_RECONNECT_DELAYS[0];
	return REMOTE_RECONNECT_DELAYS[Math.min(Math.floor(attempt), REMOTE_RECONNECT_DELAYS.length - 1)];
}

export function shouldUseRemoteFrameFallback(closedLanes: readonly boolean[]) {
	return closedLanes.length > 0 && closedLanes.every(Boolean);
}

// An HTTP long-poll can complete after a WebSocket lane has already recovered.
// A monotonically increasing generation keeps that stale response (and its
// finally-block timer) out of the next fallback cycle.
export function isCurrentRemoteFallbackGeneration(active: boolean, generation: number, currentGeneration: number) {
	return active
		&& Number.isSafeInteger(generation)
		&& Number.isSafeInteger(currentGeneration)
		&& generation === currentGeneration;
}

export function isRecoverableRemoteStatusFailure(consecutiveFailures: number) {
	return Number.isFinite(consecutiveFailures) && consecutiveFailures > 0 && consecutiveFailures < 8;
}

// Even a completely static Windows desktop publishes a liveness frame every
// five seconds. Mobile browsers can keep a WebSocket in OPEN after Wi-Fi/VPN
// routing changed, so readyState alone is not proof that the stream still has
// a working path. Leave enough margin for the heartbeat plus normal Internet
// jitter, then replace the stale sockets instead of making the user reopen the
// session manually.
export function isRemoteFrameStreamStalled(lastFrameAt: number, now: number, thresholdMs = 6_500) {
	if (!Number.isFinite(lastFrameAt) || !Number.isFinite(now) || !Number.isFinite(thresholdMs) || thresholdMs <= 0) return false;
	return now - lastFrameAt >= thresholdMs;
}
