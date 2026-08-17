import assert from "node:assert/strict";
import test from "node:test";
import { cameraKeepingPointUnderFingers, clampRemoteCamera, pointUnderScreenCoordinate } from "../src/remoteCamera.ts";

test("pinch zoom preserves the remote point below the moving finger midpoint", () => {
	const center = { x: 540, y: 360 };
	const start = { zoom: 1.25, panX: 84, panY: -31 };
	const firstMidpoint = { x: 310, y: 248 };
	const anchor = pointUnderScreenCoordinate(firstMidpoint, center, start);
	const movedMidpoint = { x: 355, y: 221 };
	const next = cameraKeepingPointUnderFingers(anchor, movedMidpoint, center, 2.1);
	const pointAfterZoom = pointUnderScreenCoordinate(movedMidpoint, center, next);

	assert.ok(Math.abs(pointAfterZoom.x - anchor.x) < 1e-9);
	assert.ok(Math.abs(pointAfterZoom.y - anchor.y) < 1e-9);
});

test("off-centre pinch does not jump its anchor to the viewport centre", () => {
	const center = { x: 600, y: 400 };
	const fingers = { x: 180, y: 140 };
	const anchor = pointUnderScreenCoordinate(fingers, center, { zoom: 1, panX: 0, panY: 0 });
	const next = cameraKeepingPointUnderFingers(anchor, fingers, center, 2);

	assert.notEqual(next.panX, 0);
	assert.notEqual(next.panY, 0);
	assert.deepEqual(pointUnderScreenCoordinate(fingers, center, next), anchor);
});

test("camera boundaries permit useful panning but prevent losing the whole desktop", () => {
	const clamped = clampRemoteCamera({ zoom: 8, panX: 99_000, panY: -99_000 }, { x: 960, y: 540 }, { x: 960, y: 540 });
	assert.equal(clamped.zoom, 4);
	assert.equal(clamped.panX, 1472);
	assert.equal(clamped.panY, -842);
});
