import assert from "node:assert/strict";
import test from "node:test";
import { LatestPointerCadence, remotePointerCadenceMillis } from "../src/remotePointerCadence.ts";

test("pointer cadence sends the leading sample and the newest trailing sample", () => {
	const cadence = new LatestPointerCadence<{ x: number }>(8);
	assert.deepEqual(cadence.offer({ x: 1 }, 100), { send: { x: 1 }, delayMs: 0 });
	assert.deepEqual(cadence.offer({ x: 2 }, 102), { send: null, delayMs: 6 });
	assert.deepEqual(cadence.offer({ x: 3 }, 105), { send: null, delayMs: 3 });
	assert.equal(cadence.take(107), null);
	assert.deepEqual(cadence.take(108), { x: 3 });
	assert.equal(cadence.take(109), null);
});

test("a stateful pointer boundary can force the latest coordinate before button input", () => {
	const cadence = new LatestPointerCadence<string>(12);
	assert.equal(cadence.offer("move-1", 1).send, "move-1");
	assert.equal(cadence.offer("move-2", 5).send, null);
	assert.equal(cadence.offer("move-3", 7).send, null);
	assert.equal(cadence.take(7, true), "move-3");
	assert.equal(cadence.take(20, true), null);
});

test("pointer cadence applies bounded back-pressure without coupling to video FPS", () => {
	assert.equal(remotePointerCadenceMillis(false, 0), 6);
	assert.equal(remotePointerCadenceMillis(true, 0), 8);
	assert.equal(remotePointerCadenceMillis(true, 16 * 1024), 12);
	assert.equal(remotePointerCadenceMillis(false, 64 * 1024), 20);
	assert.equal(remotePointerCadenceMillis(true, Number.NaN), 8);
});
