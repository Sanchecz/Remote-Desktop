import assert from "node:assert/strict";
import test from "node:test";
import { createRemoteFrameRetirementScheduler } from "../src/remoteFramePresentation.ts";

test("a replaced remote frame survives two complete paints before revocation", () => {
	let nextID = 0;
	const scheduled = new Map<number, FrameRequestCallback>();
	const revoked: string[] = [];
	const scheduler = createRemoteFrameRetirementScheduler({
		schedulePaint(callback) {
			const id = ++nextID;
			scheduled.set(id, callback);
			return id;
		},
		cancelPaint(id) { scheduled.delete(id); },
		revoke(url) { revoked.push(url); },
		paintCount: 2,
	});

	const paint = () => {
		const entry = scheduled.entries().next().value as [number, FrameRequestCallback] | undefined;
		assert.ok(entry, "a compositor paint must be scheduled");
		scheduled.delete(entry[0]);
		entry[1](performance.now());
	};

	scheduler.retire("blob:old-frame");
	paint();
	assert.deepEqual(revoked, [], "the previous JPEG remains available during the first swap paint");
	paint();
	assert.deepEqual(revoked, ["blob:old-frame"]);
	scheduler.dispose();
});

test("disposing a session releases every frame still waiting for a paint", () => {
	let callback: FrameRequestCallback | null = null;
	const revoked: string[] = [];
	const scheduler = createRemoteFrameRetirementScheduler({
		schedulePaint(next) { callback = next; return 7; },
		cancelPaint() { callback = null; },
		revoke(url) { revoked.push(url); },
		paintCount: 3,
	});

	scheduler.retire("blob:first");
	scheduler.retire("blob:second");
	scheduler.dispose();
	assert.equal(callback, null);
	assert.deepEqual(revoked.sort(), ["blob:first", "blob:second"]);
});
