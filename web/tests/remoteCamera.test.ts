import assert from "node:assert/strict";
import test from "node:test";
import { advanceRemotePinch, advanceRemoteTrackpadCursor, authoritativeRemoteFrameSize, cameraFollowingRemotePoint, cameraKeepingPointUnderFingers, canReleaseRemoteTouchSuppression, clampRemoteCamera, clampRemotePoint, classifyRemoteTouchGesture, fillRemoteFrame, fitRemoteFrame, isRemoteTwoFingerTap, pointUnderScreenCoordinate, remoteCursorVisualPoint, remoteCursorVisualPointForLayer, remotePointerTapActions, remotePointFromClient, reprojectRemotePoint, shouldPresentDecodedRemoteFrame, stabilizeRemoteTrackpadMotion, stableRemoteTrackpadDelta, stableRemoteTrackpadSamples } from "../src/remoteCamera.ts";

test("a remote pointer tap contains exactly one press and one release", () => {
	assert.deepEqual(remotePointerTapActions("left"), [
		{ action: "down", button: "left" },
		{ action: "up", button: "left" },
	]);
	assert.deepEqual(remotePointerTapActions("right"), [
		{ action: "down", button: "right" },
		{ action: "up", button: "right" },
	]);
});

test("a frame source swap keeps the last decoded coordinate space", () => {
	assert.deepEqual(
		authoritativeRemoteFrameSize(
			{ x: 0, y: 0 },
			{ x: 2256, y: 1504 },
			{ x: 1920, y: 1080 },
			{ x: 390, y: 260 },
		),
		{ x: 2256, y: 1504 },
	);
	assert.deepEqual(
		authoritativeRemoteFrameSize(
			{ x: 1920, y: 1080 },
			{ x: 2256, y: 1504 },
			{ x: 2256, y: 1504 },
			{ x: 390, y: 219 },
		),
		{ x: 1920, y: 1080 },
	);
});

test("a partially decoded frame never mixes axes from different coordinate spaces", () => {
	assert.deepEqual(
		authoritativeRemoteFrameSize(
			{ x: 2560, y: 0 },
			{ x: 2256, y: 1504 },
			{ x: 2560, y: 1440 },
			{ x: 844, y: 475 },
		),
		{ x: 2256, y: 1504 },
	);
	assert.deepEqual(
		authoritativeRemoteFrameSize(
			{ x: 0, y: 1440 },
			{ x: 0, y: 0 },
			{ x: 2560, y: 1440 },
			{ x: 844, y: 475 },
		),
		{ x: 2560, y: 1440 },
	);
	assert.deepEqual(
		authoritativeRemoteFrameSize(
			{ x: Number.NaN, y: Number.NaN },
			{ x: 0, y: 0 },
			{ x: 0, y: 0 },
			{ x: 844.4, y: 474.6 },
		),
		{ x: 844, y: 475 },
	);
});

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

test("camera boundaries permit useful panning without exposing the black canvas", () => {
	const clamped = clampRemoteCamera({ zoom: 8, panX: 99_000, panY: -99_000 }, { x: 960, y: 540 }, { x: 960, y: 540 });
	assert.equal(clamped.zoom, 4);
	assert.equal(clamped.panX, 1440);
	assert.equal(clamped.panY, -810);
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

test("portrait letterboxing remains centred instead of becoming a moving black rectangle", () => {
	const content = { x: 390, y: 219 };
	const viewport = { x: 390, y: 760 };
	const clamped = clampRemoteCamera({ zoom: 1.8, panX: 999, panY: -999 }, content, viewport);
	assert.equal(clamped.panX, 156);
	assert.equal(clamped.panY, 0, "a frame shorter than the phone viewport must not slide through the letterbox");
});

test("camera matrix never reveals canvas where the scaled desktop can cover the viewport", () => {
	for (const content of [{ x: 390, y: 219 }, { x: 591, y: 394 }, { x: 844, y: 475 }, { x: 320, y: 568 }]) {
		for (const viewport of [{ x: 390, y: 760 }, { x: 844, y: 342 }, { x: 591, y: 394 }]) {
			for (const zoom of [1, 1.1, 1.5, 2, 3, 4]) {
				const clamped = clampRemoteCamera({ zoom, panX: 1_000_000, panY: -1_000_000 }, content, viewport);
				for (const axis of ["x", "y"] as const) {
					const scaled = content[axis] * zoom;
					const pan = axis === "x" ? clamped.panX : clamped.panY;
					const limit = Math.max(0, (scaled - viewport[axis]) / 2);
					assert.ok(Math.abs(pan) <= limit + 1e-9, `${axis} escaped at ${content.x}x${content.y}, ${viewport.x}x${viewport.y}, ${zoom}x`);
					if (scaled <= viewport[axis]) assert.equal(pan, 0, `${axis} letterbox must remain centred`);
				}
			}
		}
	}
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

test("post-pinch taps stay suppressed until all fingers and the pinch are finished", () => {
	assert.equal(canReleaseRemoteTouchSuppression(2, true), false);
	assert.equal(canReleaseRemoteTouchSuppression(1, false), false);
	assert.equal(canReleaseRemoteTouchSuppression(0, true), false);
	assert.equal(canReleaseRemoteTouchSuppression(0, false), true);
	assert.equal(canReleaseRemoteTouchSuppression(Number.NaN, false), false);
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
	assert.ok(right.panX <= -190, "right-edge cursor must pan the zoomed desktop left up to the non-exposing boundary");
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

test("local cursor keeps sub-pixel progress while network packets remain integer", () => {
	const frame = { x: 3840, y: 2160 };
	let precise = { x: 1000, y: 800 };
	const visual: number[] = [];
	const packets: number[] = [];
	for (let index = 0; index < 40; index += 1) {
		precise = advanceRemoteTrackpadCursor(precise, { x: 0.13, y: 0 }, frame, { x: 915, y: 515 });
		visual.push(remoteCursorVisualPoint(precise, frame).x);
		packets.push(clampRemotePoint(precise, frame).x);
	}
	assert.ok(visual.every((value, index) => index === 0 || value > visual[index - 1]), "compositor cursor must advance on every sample");
	assert.ok(packets.every(Number.isInteger), "Agent packets must stay on integer desktop pixels");
	assert.ok(new Set(packets).size > 10, "network cursor must still make steady progress");
	assert.deepEqual(remoteCursorVisualPoint({ x: Number.NaN, y: Number.POSITIVE_INFINITY }, frame), { x: 0, y: 0 });
	assert.deepEqual(remoteCursorVisualPoint({ x: -1.25, y: 9999 }, frame), { x: 0, y: 2159 });
});

test("local cursor does not jump while a new frame geometry waits for the layer commit", () => {
	// The JPEG has already changed to 2560x1440, but the image layer is still the
	// preceding 3840x2160 React render. The visual cursor must remain at the same
	// normalized point in that old layer until the commit catches up.
	const onOldLayer = remoteCursorVisualPointForLayer(
		{ x: 1279.5, y: 719.5 },
		{ x: 2560, y: 1440 },
		{ x: 3840, y: 2160 },
	);
	assert.ok(Math.abs(onOldLayer.x - 1919.5) < 1e-9);
	assert.ok(Math.abs(onOldLayer.y - 1079.5) < 1e-9);

	const afterCommit = remoteCursorVisualPointForLayer(
		{ x: 1279.5, y: 719.5 },
		{ x: 2560, y: 1440 },
		{ x: 2560, y: 1440 },
	);
	assert.deepEqual(afterCommit, { x: 1279.5, y: 719.5 });
	assert.deepEqual(
		remoteCursorVisualPointForLayer({ x: 99, y: 49 }, { x: 200, y: 100 }, { x: 0, y: 0 }),
		{ x: 99, y: 49 },
	);
});

test("continuous arrivals cannot starve a completed frame decode", () => {
	let lastPresented = 0;
	// Frame 1 started decoding; frames 2..20 arrive before it completes. The
	// arrival counter must not suppress frame 1, otherwise a busy stream freezes.
	const latestArrival = 20;
	assert.equal(shouldPresentDecodedRemoteFrame(1, lastPresented), true);
	lastPresented = 1;
	assert.equal(latestArrival > lastPresented, true);
	// The one-slot queue next decodes arrival 20 and older/duplicate callbacks are
	// still rejected.
	assert.equal(shouldPresentDecodedRemoteFrame(latestArrival, lastPresented), true);
	lastPresented = latestArrival;
	assert.equal(shouldPresentDecodedRemoteFrame(19, lastPresented), false);
	assert.equal(shouldPresentDecodedRemoteFrame(20, lastPresented), false);
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

test("mobile trackpad rejects viewport-coordinate teleports without slowing real swipes", () => {
	assert.deepEqual(stableRemoteTrackpadDelta({ x: 18, y: -7 }, { x: 915, y: 515 }), { x: 18, y: -7 });
	const coalesced = stableRemoteTrackpadDelta({ x: 140, y: 0 }, { x: 915, y: 515 });
	assert.ok(coalesced.x > 35 && coalesced.x <= 37);
	assert.deepEqual(stableRemoteTrackpadDelta({ x: -700, y: 420 }, { x: 915, y: 515 }), { x: 0, y: 0 });
	assert.deepEqual(stableRemoteTrackpadDelta({ x: Number.NaN, y: Number.POSITIVE_INFINITY }, { x: 390, y: 219 }), { x: 0, y: 0 });
});

test("trackpad movement stays bounded and monotonic across phone and remote-screen matrices", () => {
	const frames = [
		{ x: 1366, y: 768 }, { x: 1920, y: 1080 }, { x: 2256, y: 1504 },
		{ x: 2560, y: 1440 }, { x: 3440, y: 1440 }, { x: 3840, y: 2160 },
		{ x: 5120, y: 1440 }, { x: 1080, y: 1920 },
	];
	const renderedSizes = [
		{ x: 320, y: 568 }, { x: 390, y: 786 }, { x: 740, y: 328 },
		{ x: 844, y: 342 }, { x: 915, y: 364 }, { x: 1280, y: 576 },
	];
	let seed = 0x13579bdf;
	const random = () => {
		seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
		return seed / 0x100000000;
	};
	for (const frame of frames) {
		for (const rendered of renderedSizes) {
			let cursor = { x: frame.x / 2, y: frame.y / 2 };
			for (let index = 0; index < 500; index += 1) {
				const direction = index % 4;
				const magnitude = 0.25 + random() * 38;
				const raw = direction === 0 ? { x: magnitude, y: random() * 4 - 2 }
					: direction === 1 ? { x: -magnitude, y: random() * 4 - 2 }
						: direction === 2 ? { x: random() * 4 - 2, y: magnitude }
							: { x: random() * 4 - 2, y: -magnitude };
				const filtered = stableRemoteTrackpadDelta(raw, rendered);
				const next = advanceRemoteTrackpadCursor(cursor, filtered, frame, rendered);
				assert.ok(next.x >= 0 && next.x <= frame.x - 1, `x escaped ${frame.x}x${frame.y} in ${rendered.x}x${rendered.y}`);
				assert.ok(next.y >= 0 && next.y <= frame.y - 1, `y escaped ${frame.x}x${frame.y} in ${rendered.x}x${rendered.y}`);
				if (raw.x > 2 && cursor.x < frame.x - 1) assert.ok(next.x >= cursor.x, "positive x motion reversed");
				if (raw.x < -2 && cursor.x > 0) assert.ok(next.x <= cursor.x, "negative x motion reversed");
				if (raw.y > 2 && cursor.y < frame.y - 1) assert.ok(next.y >= cursor.y, "positive y motion reversed");
				if (raw.y < -2 && cursor.y > 0) assert.ok(next.y <= cursor.y, "negative y motion reversed");
				cursor = next;
			}
		}
	}
});

test("coalesced mobile samples preserve a fast swipe but isolate a viewport rebase", () => {
	const normal = stableRemoteTrackpadSamples(
		{ x: 100, y: 200 },
		[{ x: 112, y: 203 }, { x: 128, y: 207 }, { x: 149, y: 211 }],
		{ x: 915, y: 515 },
	);
	assert.deepEqual(normal.deltas, [{ x: 12, y: 3 }, { x: 16, y: 4 }, { x: 21, y: 4 }]);
	assert.deepEqual(normal.last, { x: 149, y: 211 });
	assert.equal(normal.lastTime, 0);

	const rebased = stableRemoteTrackpadSamples(
		{ x: 149, y: 211 },
		[{ x: 810, y: 620 }, { x: 817, y: 624 }],
		{ x: 915, y: 515 },
	);
	assert.deepEqual(rebased.deltas, [{ x: 7, y: 4 }], "the first post-rebase real movement must not be lost");
	assert.deepEqual(rebased.last, { x: 817, y: 624 });

	const myNotebookInsetRebase = stableRemoteTrackpadSamples(
		{ x: 186, y: 512 },
		[{ x: 276, y: 486 }, { x: 281, y: 489 }, { x: 288, y: 491 }],
		{ x: 591, y: 394 },
	);
	assert.deepEqual(
		myNotebookInsetRebase.deltas,
		[{ x: 5, y: 3 }, { x: 7, y: 2 }],
		"a browser-inset rebase on the 2256x1504 notebook must not teleport the cursor",
	);

	const solitaryInsetCandidate = stableRemoteTrackpadSamples(
		{ x: 186, y: 512 },
		[{ x: 276, y: 486 }],
		{ x: 591, y: 394 },
	);
	assert.equal(solitaryInsetCandidate.deltas.length, 1, "a solitary sample must reach the cross-event stabilizer instead of disappearing");
	assert.ok(Math.hypot(solitaryInsetCandidate.deltas[0].x, solitaryInsetCandidate.deltas[0].y) <= 52.01, "the candidate remains safely bounded");
	assert.deepEqual(solitaryInsetCandidate.last, { x: 276, y: 486 });
	const motionAfterSolitaryCandidate = stableRemoteTrackpadSamples(
		solitaryInsetCandidate.last,
		[{ x: 281, y: 489 }],
		{ x: 591, y: 394 },
	);
	assert.deepEqual(motionAfterSolitaryCandidate.deltas, [{ x: 5, y: 3 }], "movement must resume immediately after a solitary candidate");

	const realFastSwipe = stableRemoteTrackpadSamples(
		{ x: 186, y: 512 },
		[{ x: 266, y: 500 }, { x: 336, y: 491 }, { x: 392, y: 485 }],
		{ x: 591, y: 394 },
	);
	assert.equal(realFastSwipe.deltas.length, 3, "sustained fast motion must not be mistaken for a viewport rebase");
	assert.ok(realFastSwipe.deltas.every((delta) => delta.x > 0));

	const slowBrowserSwipe = stableRemoteTrackpadSamples(
		{ x: 100, y: 100 },
		[{ x: 172, y: 100, time: 190 }],
		{ x: 591, y: 394 },
		100,
	);
	assert.equal(slowBrowserSwipe.deltas.length, 1, "a deliberate sample after a long browser pause must not be mistaken for an inset rebase");
	assert.ok(slowBrowserSwipe.deltas[0].x > 0);
	assert.equal(slowBrowserSwipe.lastTime, 190);

	const instantInsetJump = stableRemoteTrackpadSamples(
		{ x: 100, y: 100 },
		[{ x: 172, y: 100, time: 116 }],
		{ x: 591, y: 394 },
		100,
	);
	assert.equal(instantInsetJump.deltas.length, 1, "a fast solitary swipe must not be discarded before cross-event classification");
	const heldInsetJump = stabilizeRemoteTrackpadMotion(null, instantInsetJump.deltas, { x: 591, y: 394 }, 16);
	assert.deepEqual(heldInsetJump.deltas, [], "a rapid ambiguous sample is held for one short look-ahead");
	assert.notEqual(heldInsetJump.pending, null);
});

test("a rapid sub-threshold viewport jump and return never reaches the remote cursor", () => {
	const rendered = { x: 591, y: 394 };
	const jump = stabilizeRemoteTrackpadMotion(null, [{ x: 31, y: -9 }], rendered, 16);
	assert.deepEqual(jump.deltas, []);
	assert.deepEqual(jump.pending, { x: 31, y: -9 });

	const returned = stabilizeRemoteTrackpadMotion(jump.pending, [{ x: -30, y: 8 }], rendered, 17);
	assert.deepEqual(returned.deltas, [], "neither half of a browser jump-return pair may be rendered or sent");
	assert.equal(returned.pending, null);
});

test("large genuine trackpad motion is ordered and recoverable after the look-ahead", () => {
	const rendered = { x: 591, y: 394 };
	const first = stabilizeRemoteTrackpadMotion(null, [{ x: 30, y: 3 }], rendered, 16);
	const second = stabilizeRemoteTrackpadMotion(first.pending, [{ x: 34, y: 4 }], rendered, 16);
	assert.deepEqual(second.deltas, [{ x: 30, y: 3 }], "continued movement confirms the previous sample in order");
	assert.deepEqual(second.pending, { x: 34, y: 4 });

	const slow = stabilizeRemoteTrackpadMotion(second.pending, [{ x: 45, y: 2 }], rendered, 90);
	assert.deepEqual(slow.deltas, [{ x: 34, y: 4 }, { x: 45, y: 2 }], "a long-pause deliberate swipe must not remain buffered");
	assert.equal(slow.pending, null);

	assert.deepEqual(
		stabilizeRemoteTrackpadMotion(null, [{ x: 4, y: 2 }, { x: 5, y: 2 }], rendered, 16),
		{ deltas: [{ x: 4, y: 2 }, { x: 5, y: 2 }], pending: null },
		"ordinary coalesced samples retain the zero-latency path",
	);
});

test("fit mode keeps the entire desktop visible in phone portrait and landscape", () => {
	for (const frame of [{ x: 1366, y: 768 }, { x: 1920, y: 1080 }, { x: 2256, y: 1504 }, { x: 2560, y: 1440 }, { x: 2560, y: 1600 }, { x: 3440, y: 1440 }, { x: 3840, y: 2160 }, { x: 1080, y: 1920 }]) {
		for (const viewport of [{ x: 390, y: 786 }, { x: 844, y: 342 }, { x: 360, y: 640 }, { x: 915, y: 364 }, { x: 1280, y: 576 }, { x: 740, y: 328 }]) {
			const fitted = fitRemoteFrame(frame, viewport);
			assert.ok(fitted.x <= viewport.x, `${frame.x}x${frame.y} exceeds portrait/landscape width`);
			assert.ok(fitted.y <= viewport.y, `${frame.x}x${frame.y} exceeds portrait/landscape height`);
			assert.ok(Math.abs(fitted.x / fitted.y - frame.x / frame.y) < 0.02, "fit must preserve the desktop aspect ratio");
		}
	}
});

test("fill mode covers the complete phone and desktop canvas without distorting the frame", () => {
	for (const frame of [{ x: 1366, y: 768 }, { x: 1920, y: 1080 }, { x: 2256, y: 1504 }, { x: 3440, y: 1440 }, { x: 1080, y: 1920 }]) {
		for (const viewport of [{ x: 390, y: 786 }, { x: 844, y: 342 }, { x: 1440, y: 900 }, { x: 2560, y: 1080 }]) {
			const filled = fillRemoteFrame(frame, viewport);
			assert.ok(filled.x >= viewport.x, `${frame.x}x${frame.y} leaves horizontal bands in ${viewport.x}x${viewport.y}`);
			assert.ok(filled.y >= viewport.y, `${frame.x}x${frame.y} leaves vertical bands in ${viewport.x}x${viewport.y}`);
			assert.ok(Math.abs(filled.x / filled.y - frame.x / frame.y) < 0.02, "fill must preserve the desktop aspect ratio");
		}
	}
});

test("fill mode camera can expose every cropped edge without uncovering the canvas", () => {
	const viewport = { x: 390, y: 786 };
	const filled = fillRemoteFrame({ x: 1920, y: 1080 }, viewport);
	const rightEdge = clampRemoteCamera({ zoom: 1, panX: -10000, panY: 10000 }, filled, viewport);
	assert.equal(rightEdge.panX, -(filled.x - viewport.x) / 2);
	assert.equal(rightEdge.panY, 0);
	const zoomed = clampRemoteCamera({ zoom: 2, panX: 10000, panY: -10000 }, filled, viewport);
	assert.equal(zoomed.panX, (filled.x * 2 - viewport.x) / 2);
	assert.equal(zoomed.panY, -(filled.y * 2 - viewport.y) / 2);
});

test("direct touch remains pixel-accurate across desktop and phone aspect-ratio matrices", () => {
	const frames = [
		{ x: 1024, y: 768 },
		{ x: 1366, y: 768 },
		{ x: 1920, y: 1080 },
		{ x: 2256, y: 1504 },
		{ x: 2560, y: 1440 },
		{ x: 2560, y: 1600 },
		{ x: 3440, y: 1440 },
		{ x: 3840, y: 2160 },
		{ x: 5120, y: 1440 },
		{ x: 1080, y: 1920 },
	];
	const viewports = [
		{ x: 320, y: 568 },
		{ x: 390, y: 786 },
		{ x: 844, y: 342 },
		{ x: 915, y: 364 },
		{ x: 1280, y: 576 },
	];
	for (const frame of frames) {
		for (const viewport of viewports) {
			const fitted = fitRemoteFrame(frame, viewport);
			const imageRect = {
				left: 17 + (viewport.x - fitted.x) / 2,
				top: 23 + (viewport.y - fitted.y) / 2,
				width: fitted.x,
				height: fitted.y,
			};
			for (const xRatio of [0, 0.001, 0.1, 0.5, 0.9, 0.999, 1]) {
				for (const yRatio of [0, 0.1, 0.5, 0.9, 1]) {
					const mapped = remotePointFromClient({
						x: imageRect.left + imageRect.width * xRatio,
						y: imageRect.top + imageRect.height * yRatio,
					}, imageRect, frame);
					const expected = {
						x: Math.min(frame.x - 1, Math.round(frame.x * xRatio)),
						y: Math.min(frame.y - 1, Math.round(frame.y * yRatio)),
					};
					assert.deepEqual(mapped, expected, `${frame.x}x${frame.y} in ${viewport.x}x${viewport.y} at ${xRatio},${yRatio}`);
				}
			}
		}
	}
});

test("cursor remains on the same normalized desktop point when stream geometry changes", () => {
	const original = { x: 1536, y: 864 };
	const interactive = reprojectRemotePoint(original, { x: 1920, y: 1080 }, { x: 1600, y: 900 });
	assert.ok(Math.abs(interactive.x / 1599 - original.x / 1919) < 1e-12);
	assert.ok(Math.abs(interactive.y / 899 - original.y / 1079) < 1e-12);
	const restored = reprojectRemotePoint(interactive, { x: 1600, y: 900 }, { x: 3840, y: 2160 });
	assert.ok(Math.abs(restored.x / 3839 - original.x / 1919) < 1e-12);
	assert.ok(Math.abs(restored.y / 2159 - original.y / 1079) < 1e-12);
});

test("coordinate reprojection is bounded for nonstandard and portrait frames", () => {
	assert.deepEqual(reprojectRemotePoint({ x: -50, y: 9999 }, { x: 2256, y: 1504 }, { x: 1080, y: 1920 }), { x: 0, y: 1919 });
	const ultrawide = reprojectRemotePoint({ x: 1720, y: 720 }, { x: 3440, y: 1440 }, { x: 1920, y: 804 });
	assert.ok(Math.abs(ultrawide.x / 1919 - 1720 / 3439) < 1e-12);
	assert.ok(Math.abs(ultrawide.y / 803 - 720 / 1439) < 1e-12);
});

test("cursor reprojection has no cumulative drift across repeated profile and orientation changes", () => {
	const frames = [
		{ x: 2256, y: 1504 },
		{ x: 1600, y: 1067 },
		{ x: 3440, y: 1440 },
		{ x: 1080, y: 1920 },
		{ x: 3840, y: 2160 },
		{ x: 1366, y: 768 },
	];
	for (const normalized of [0, 0.001, 0.125, 0.5, 0.87321, 0.999, 1]) {
		let frame = frames[0];
		let point = { x: normalized * (frame.x - 1), y: (1 - normalized) * (frame.y - 1) };
		for (let cycle = 0; cycle < 100; cycle += 1) {
			const nextFrame = frames[(cycle + 1) % frames.length];
			point = reprojectRemotePoint(point, frame, nextFrame);
			frame = nextFrame;
			assert.ok(point.x >= 0 && point.x <= frame.x - 1);
			assert.ok(point.y >= 0 && point.y <= frame.y - 1);
			assert.ok(Math.abs(point.x / Math.max(1, frame.x - 1) - normalized) < 1e-10);
			assert.ok(Math.abs(point.y / Math.max(1, frame.y - 1) - (1 - normalized)) < 1e-10);
		}
	}
});
