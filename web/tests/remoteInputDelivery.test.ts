import assert from "node:assert/strict";
import test from "node:test";
import { bindRemoteInputCoordinates, remoteInputAckID, remoteInputBatchID, remoteInputClientID, restoreRemoteInputBatch, shouldRetryRemoteInputDelivery, takePendingRemoteInputBatches } from "../src/remoteInputDelivery.ts";

test("client input ids remain compact and transport-safe", () => {
	assert.equal(remoteInputClientID("viewer/a b", 12), "viewer_a_b:12");
	assert.match(remoteInputClientID("v", -2), /^v:1$/);
	assert.equal(remoteInputBatchID("viewer/a b", 8), "viewer_a_b:b8");
});

test("binding frame geometry keeps the stable client input id", () => {
	assert.deepEqual(
		bindRemoteInputCoordinates({ type: "pointer", action: "move", clientInputId: "v:7" }, 2256, 1504),
		{ type: "pointer", action: "move", clientInputId: "v:7", coordinateWidth: 2256, coordinateHeight: 1504 },
	);
});

test("websocket acknowledgements are parsed narrowly", () => {
	assert.equal(remoteInputAckID(JSON.stringify({ inputAck: "v:b9" })), "v:b9");
	assert.equal(remoteInputAckID(JSON.stringify({ inputAck: 9 })), "");
	assert.equal(remoteInputAckID(new ArrayBuffer(4)), "");
});

test("pending websocket batches restore in insertion order", () => {
	const socketA = {};
	const socketB = {};
	const pending = new Map([
		["v:b1", { events: [{ type: "text", text: "A", clientInputId: "v:1" }], socket: socketA, expiresAt: 100 }],
		["v:b2", { events: [{ type: "text", text: "B", clientInputId: "v:2" }], socket: socketB, expiresAt: 200 }],
		["v:b3", { events: [{ type: "key", action: "up", keyCode: 16, clientInputId: "v:3" }], socket: socketA, expiresAt: 300 }],
	]);
	const restored = takePendingRemoteInputBatches(pending, (batch) => batch.socket === socketA);
	assert.deepEqual(restored.map((event) => event.clientInputId), ["v:1", "v:3"]);
	assert.deepEqual([...pending.keys()], ["v:b2"]);
});

test("failed fallback restores stateful input in exact order", () => {
	const retry = [
		{ type: "text", text: "RemoteIt", clientInputId: "v:1" },
		{ type: "key", action: "up", keyCode: 16, clientInputId: "v:2" },
	];
	const queued = [{ type: "pointer", action: "down", button: "left", clientInputId: "v:3" }];
	assert.deepEqual(restoreRemoteInputBatch(retry, queued), [...retry, ...queued]);
});

test("cancelled input from a disposed device cannot enter the next device queue", () => {
	assert.equal(shouldRetryRemoteInputDelivery("session-b", "session-a"), false);
	assert.equal(shouldRetryRemoteInputDelivery("", "session-a"), false);
	assert.equal(shouldRetryRemoteInputDelivery("session-a", "session-a"), true);
});

test("only adjacent free moves coalesce when a batch is retried", () => {
	const restored = restoreRemoteInputBatch(
		[{ type: "pointer", action: "move", x: 10, clientInputId: "v:1" }],
		[
			{ type: "pointer", action: "move", x: 20, clientInputId: "v:2" },
			{ type: "pointer", action: "down", button: "left", x: 20, clientInputId: "v:3" },
			{ type: "pointer", action: "move", x: 30, clientInputId: "v:4" },
		],
	);
	assert.deepEqual(restored.map((event) => event.clientInputId), ["v:2", "v:3", "v:4"]);
});
