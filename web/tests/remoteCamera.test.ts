import assert from "node:assert/strict";
import test from "node:test";
import { advanceRemotePinch, advanceRemoteTrackpadCursor, cameraFollowingRemotePoint, cameraKeepingPointUnderFingers, clampRemoteCamera, clampRemotePoint, classifyRemoteTouchGesture, fitRemoteFrame, isRemoteTwoFingerTap, pointUnderScreenCoordinate, remotePointFromClient } from "../src/remoteCamera.ts";

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
	assert.equal(clamped.panX, 2336);
	assert.equal(clamped.panY, -1286);
});

test("successive pinch samples preserve their moving midpoint without recentering", () => {
	const center = { x: 600, y: 400 };
	const start = { zoom: 1.1, panX: -42, panY: 18 };
	const firstMidpoint = { x: 210, y: 180 };
	const firstAnchor = pointUnderScreenCoordinate(firstMidpoint, center, start);
	const first = advanceRemotePinch(start, firstMidpoint, { x: 228, y: 172 }, center, 120, 138);
	assert.deepEqual(pointUnderScreenCoordinate({ x: 228, y: 172 }, center, first), firstAnchor);

	const secondAnchor = pointUnderScreenCoordinate({ x: 228, y: 172 }, center, first);
	const second = advanceRemotePinch(first, { x: 228, y: 172 }, { x: 251, y: 190 }, center, 138, 162);
	const pointAfterSecondSample = pointUnderScreenCoordinate({ x: 251, y: 190 }, center, second);
	assert.ok(Math.abs(pointAfterSecondSample.x - secondAnchor.x) < 1e-9);
	assert.ok(Math.abs(pointAfterSecondSample.y - secondAnchor.y) < 1e-9);
	assert.notEqual(second.panX, 0);
	assert.notEqual(second.panY, 0);
});

test("portrait letterboxing does not force an off-centre pinch back to the middle", () => {
	// 390x219 is a 16:9 desktop fitted into a 390x760 portrait viewport.
	const content = { x: 390, y: 219 };
	const viewport = { x: 390, y: 760 };
	const center = { x: viewport.x / 2, y: viewport.y / 2 };
	const firstMidpoint = { x: 116, y: 315 };
	const anchor = pointUnderScreenCoordinate(firstMidpoint, center, { zoom: 1, panX: 0, panY: 0 });
	const zoomed = advanceRemotePinch({ zoom: 1, panX: 0, panY: 0 }, firstMidpoint, firstMidpoint, center, 100, 180);
	const clamped = clampRemoteCamera(zoomed, content, viewport);
	const pointAfterClamp = pointUnderScreenCoordinate(firstMidpoint, center, clamped);

	assert.ok(Math.abs(pointAfterClamp.x - anchor.x) < 1e-9);
	assert.ok(Math.abs(pointAfterClamp.y - anchor.y) < 1e-9);
});

test("portrait pinch can keep the lower desktop edge under the fingers", () => {
	const content = { x: 390, y: 219 };
	const viewport = { x: 390, y: 760 };
	const center = { x: viewport.x / 2, y: viewport.y / 2 };
	const fingers = { x: 250, y: 675 };
	const anchor = pointUnderScreenCoordinate(fingers, center, { zoom: 1, panX: 0, panY: 0 });
	const zoomed = advanceRemotePinch({ zoom: 1, panX: 0, panY: 0 }, fingers, fingers, center, 100, 230);
	const clamped = clampRemoteCamera(zoomed, content, viewport);
	const pointAfterClamp = pointUnderScreenCoordinate(fingers, center, clamped);

	assert.ok(Math.abs(pointAfterClamp.x - anchor.x) < 1e-9);
	assert.ok(Math.abs(pointAfterClamp.y - anchor.y) < 1e-9);
	assert.ok(clamped.panY < -100, "bottom-focused pinch must not be recentered");
});

test("mobile trackpad distinguishes zoom, scroll and two-finger right click", () => {
	assert.equal(classifyRemoteTouchGesture("pending", true, 1.08, 2), "zoom");
	assert.equal(classifyRemoteTouchGesture("pending", true, 1.01, 12), "scroll");
	assert.equal(classifyRemoteTouchGesture("scroll", true, 1.08, 22), "scroll");
	assert.equal(classifyRemoteTouchGesture("zoom", true, 1, 40), "zoom");
	assert.equal(classifyRemoteTouchGesture("pending", false, 1, 0), "zoom");
	assert.equal(isRemoteTwoFingerTap("pending", true, false, 180, 3), true);
	assert.equal(isRemoteTwoFingerTap("zoom", true, false, 180, 3), false);
	assert.equal(isRemoteTwoFingerTap("pending", true, true, 180, 3), false);
	assert.equal(isRemoteTwoFingerTap("pending", true, false, 600, 3), false);
});

test("direct touch maps the transformed frame and clamps black letterbox areas", () => {
	const frame = { x: 1920, y: 1080 };
	const rect = { left: 20, top: 240, width: 350, height: 197 };
	assert.deepEqual(remotePointFromClient({ x: 195, y: 338.5 }, rect, frame), { x: 960, y: 540 });
	assert.deepEqual(remotePointFromClient({ x: 195, y: 80 }, rect, frame), { x: 960, y: 0 });
	assert.deepEqual(remotePointFromClient({ x: 500, y: 700 }, rect, frame), { x: 1919, y: 1079 });
});

test("zoomed mobile cursor auto-pans only when it reaches a viewport edge", () => {
	const frame = { x: 1920, y: 1080 };
	const fitted = { x: 390, y: 219 };
	const viewport = { x: 390, y: 760 };
	const camera = { zoom: 2, panX: 0, panY: 0 };
	const centre = cameraFollowingRemotePoint(camera, { x: 960, y: 540 }, frame, fitted, viewport);
	assert.deepEqual(centre, camera);
	const right = cameraFollowingRemotePoint(camera, { x: 1919, y: 540 }, frame, fitted, viewport);
	assert.ok(right.panX < -200, "right-edge cursor must pan the zoomed desktop left");
	assert.equal(right.panY, 0);
});

test("mobile trackpad preserves fractional movement instead of sticking between pixels", () => {
	const frame = { x: 1920, y: 1080 };
	const rendered = { x: 844, y: 475 };
	let precise = { x: 960, y: 540 };
	const packets: Array<{ x: number; y: number }> = [];
	for (let index = 0; index < 12; index += 1) {
		precise = advanceRemoteTrackpadCursor(precise, { x: 0.25, y: 0.15 }, frame, rendered);
		packets.push(clampRemotePoint(precise, frame));
	}
	assert.ok(precise.x > 963, "fractional samples must accumulate in the precise cursor");
	assert.ok(new Set(packets.map((packet) => `${packet.x}:${packet.y}`)).size > 2, "rounded network packets must advance smoothly");
});

test("mobile trackpad is precise for slow motion, accelerates long swipes and stays in frame", () => {
	const frame = { x: 3840, y: 2160 };
	const rendered = { x: 915, y: 515 };
	const slow = advanceRemoteTrackpadCursor({ x: 1000, y: 800 }, { x: 1, y: 0 }, frame, rendered);
	const fast = advanceRemoteTrackpadCursor({ x: 1000, y: 800 }, { x: 25, y: 0 }, frame, rendered);
	assert.ok(slow.x > 1000 && slow.x < 1000 + frame.x / rendered.x, "slow motion must remain below one physical screen pixel per CSS pixel");
	assert.ok((fast.x - 1000) / 25 > slow.x - 1000, "a long swipe must receive smooth acceleration");
	assert.deepEqual(
		advanceRemoteTrackpadCursor({ x: 1, y: 1 }, { x: -10_000, y: 10_000 }, frame, rendered),
		{ x: 0, y: 2159 },
	);
});

test("fit mode keeps the entire desktop visible in phone portrait and landscape", () => {
	for (const frame of [{ x: 1920, y: 1080 }, { x: 2560, y: 1440 }, { x: 3840, y: 2160 }]) {
		for (const viewport of [{ x: 390, y: 786 }, { x: 844, y: 342 }, { x: 360, y: 640 }, { x: 915, y: 364 }, { x: 1280, y: 576 }, { x: 740, y: 328 }]) {
			const fitted = fitRemoteFrame(frame, viewport);
			assert.ok(fitted.x <= viewport.x, `${frame.x}x${frame.y} exceeds portrait/landscape width`);
			assert.ok(fitted.y <= viewport.y, `${frame.x}x${frame.y} exceeds portrait/landscape height`);
			assert.ok(Math.abs(fitted.x / fitted.y - frame.x / frame.y) < 0.02, "fit must preserve the desktop aspect ratio");
		}
	}
});
