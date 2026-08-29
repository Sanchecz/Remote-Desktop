export const REMOTE_RECONNECT_DELAYS = [250, 500, 1_000, 2_000, 4_000, 8_000] as const;

export function remoteReconnectDelay(attempt: number) {
	if (!Number.isFinite(attempt) || attempt <= 0) return REMOTE_RECONNECT_DELAYS[0];
	return REMOTE_RECONNECT_DELAYS[Math.min(Math.floor(attempt), REMOTE_RECONNECT_DELAYS.length - 1)];
}

export function shouldUseRemoteFrameFallback(closedLanes: readonly boolean[]) {
	return closedLanes.length > 0 && closedLanes.every(Boolean);
}

export function isRecoverableRemoteStatusFailure(consecutiveFailures: number) {
	return Number.isFinite(consecutiveFailures) && consecutiveFailures > 0 && consecutiveFailures < 8;
}
