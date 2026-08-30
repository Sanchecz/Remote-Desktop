import assert from "node:assert/strict";
import test from "node:test";
import { isCurrentRemoteFallbackGeneration, isRecoverableRemoteStatusFailure, isRemoteFrameStreamStalled, remoteReconnectDelay, shouldUseRemoteFrameFallback } from "../src/remoteReconnect.ts";

test("reconnect delay backs off quickly and remains bounded", () => {
	assert.deepEqual([-1, 0, 1, 2, 3, 4, 5, 100].map(remoteReconnectDelay), [250, 250, 500, 1_000, 2_000, 4_000, 8_000, 8_000]);
});

test("HTTP frame fallback starts only after every WebSocket lane is closed", () => {
	assert.equal(shouldUseRemoteFrameFallback([true, true, true]), true);
	assert.equal(shouldUseRemoteFrameFallback([true, false, true]), false);
	assert.equal(shouldUseRemoteFrameFallback([]), false);
});

test("brief status failures remain recoverable but a long outage is surfaced", () => {
	assert.equal(isRecoverableRemoteStatusFailure(1), true);
	assert.equal(isRecoverableRemoteStatusFailure(7), true);
	assert.equal(isRecoverableRemoteStatusFailure(8), false);
});

test("stale HTTP fallback responses cannot leak across WebSocket recovery", () => {
	assert.equal(isCurrentRemoteFallbackGeneration(true, 3, 3), true);
	assert.equal(isCurrentRemoteFallbackGeneration(false, 3, 3), false);
	assert.equal(isCurrentRemoteFallbackGeneration(true, 2, 3), false);
	assert.equal(isCurrentRemoteFallbackGeneration(true, 3, 4), false);
});

test("an OPEN but silent stream is replaced after the static-frame heartbeat margin", () => {
	assert.equal(isRemoteFrameStreamStalled(1_000, 8_999), false);
	assert.equal(isRemoteFrameStreamStalled(1_000, 9_000), true);
	assert.equal(isRemoteFrameStreamStalled(Number.NaN, 9_000), false);
	assert.equal(isRemoteFrameStreamStalled(1_000, 9_000, 0), false);
});
