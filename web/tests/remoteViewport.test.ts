import assert from "node:assert/strict";
import test from "node:test";
import { REMOTE_VIEWPORT_SETTLE_DELAYS, remoteFullscreenScaleMode, remoteViewportChanged, remoteViewportWithStableOrientation, resolveRemoteLayoutLandscape, resolveRemoteViewport, shouldApplyRemoteOrientationTransition, shouldRebaseRemotePointerViewport, shouldUseCompactRemoteControls, shouldUseRemoteTrackpad } from "../src/remoteViewport.ts";

test("Chromium and Android use the visible viewport instead of the stale layout viewport", () => {
	assert.deepEqual(resolveRemoteViewport({
		innerWidth: 412,
		innerHeight: 915,
		visualViewport: { offsetLeft: 0, offsetTop: 38, width: 915, height: 374 },
	}), { left: 0, top: 38, width: 915, height: 374, landscape: true });
});

test("Firefox fallback works when Visual Viewport is unavailable", () => {
	assert.deepEqual(resolveRemoteViewport({ innerWidth: 844, innerHeight: 390, documentWidth: 390, documentHeight: 844 }), {
		left: 0,
		top: 0,
		width: 844,
		height: 390,
		landscape: true,
	});
});

test("Safari page offsets and fractional safe viewport values stay stable", () => {
	assert.deepEqual(resolveRemoteViewport({
		innerWidth: 932,
		innerHeight: 430,
		visualViewport: { offsetLeft: 2.2, offsetTop: 4.7, width: 927.6, height: 420.4 },
	}), { left: 2, top: 5, width: 928, height: 420, landscape: true });
});

test("portrait to landscape rotation is detected after the browser finishes its second phase", () => {
	const portrait = resolveRemoteViewport({ innerWidth: 390, innerHeight: 844, visualViewport: null });
	const stale = resolveRemoteViewport({ innerWidth: 844, innerHeight: 390, visualViewport: { offsetLeft: 0, offsetTop: 0, width: 390, height: 844 } });
	const landscape = resolveRemoteViewport({ innerWidth: 844, innerHeight: 390, visualViewport: { offsetLeft: 0, offsetTop: 0, width: 844, height: 390 } });
	assert.equal(stale.landscape, false);
	assert.equal(landscape.landscape, true);
	assert.equal(remoteViewportChanged(portrait, landscape), true);
	assert.deepEqual(REMOTE_VIEWPORT_SETTLE_DELAYS, [0, 60, 180, 360, 700]);
});

test("an Android IME resize is not mistaken for a physical rotation", () => {
	assert.equal(shouldApplyRemoteOrientationTransition(false, true, true), false);
	assert.equal(shouldApplyRemoteOrientationTransition(false, true, false), true);
	assert.equal(shouldApplyRemoteOrientationTransition(false, false, false), false);
	// A real orientationchange remains authoritative even if the keyboard was open.
	assert.equal(shouldApplyRemoteOrientationTransition(false, true, true, true), true);
	const imeViewport = resolveRemoteViewport({
		innerWidth: 412,
		innerHeight: 915,
		visualViewport: { offsetLeft: 0, offsetTop: 0, width: 412, height: 336 },
	});
	assert.equal(imeViewport.landscape, true);
	assert.equal(remoteViewportWithStableOrientation(imeViewport, false, true).landscape, false);
	assert.equal(remoteViewportWithStableOrientation(imeViewport, false, false).landscape, true);
});

test("layout evidence distinguishes an IME resize from a real rotation without orientationchange", () => {
	const imeViewport = resolveRemoteViewport({
		innerWidth: 390,
		innerHeight: 844,
		documentWidth: 390,
		documentHeight: 844,
		visualViewport: { offsetLeft: 0, offsetTop: 0, width: 390, height: 330 },
	});
	const portraitLayout = resolveRemoteLayoutLandscape({ innerWidth: 390, innerHeight: 844, documentWidth: 390, documentHeight: 844 });
	assert.equal(portraitLayout, false);
	assert.equal(shouldApplyRemoteOrientationTransition(false, imeViewport.landscape, true, false, portraitLayout), false);
	assert.equal(remoteViewportWithStableOrientation(imeViewport, false, true, portraitLayout).landscape, false);

	const rotatedViewport = resolveRemoteViewport({
		innerWidth: 844,
		innerHeight: 390,
		documentWidth: 844,
		documentHeight: 390,
		visualViewport: { offsetLeft: 0, offsetTop: 0, width: 844, height: 390 },
	});
	const landscapeLayout = resolveRemoteLayoutLandscape({ innerWidth: 844, innerHeight: 390, documentWidth: 844, documentHeight: 390 });
	assert.equal(landscapeLayout, true);
	assert.equal(shouldApplyRemoteOrientationTransition(false, rotatedViewport.landscape, true, false, landscapeLayout), true);
	assert.equal(remoteViewportWithStableOrientation(rotatedViewport, false, true, landscapeLayout).landscape, true);
});

test("a two-phase browser layout waits for consistent rotation evidence", () => {
	assert.equal(resolveRemoteLayoutLandscape({ innerWidth: 844, innerHeight: 390, documentWidth: 390, documentHeight: 844 }), null);
	assert.equal(shouldApplyRemoteOrientationTransition(false, true, true, false, null), false);
});

test("invalid embedded viewport measurements never collapse the session", () => {
	assert.deepEqual(resolveRemoteViewport({
		innerWidth: 0,
		innerHeight: Number.NaN,
		documentWidth: 360,
		documentHeight: 640,
		visualViewport: { offsetLeft: -4, offsetTop: -2, width: 0, height: Number.NaN },
	}), { left: 0, top: 0, width: 360, height: 640, landscape: false });
});

test("phone-sized landscape sessions use compact controls even when a WebView reports a fine pointer", () => {
	assert.equal(shouldUseCompactRemoteControls(false, { width: 915, height: 374 }), true);
	assert.equal(shouldUseCompactRemoteControls(false, { width: 740, height: 328 }), true);
	assert.equal(shouldUseCompactRemoteControls(false, { width: 1440, height: 900 }), false);
	assert.equal(shouldUseCompactRemoteControls(true, { width: 1920, height: 1080 }), true);
});

test("compact touch viewers keep relative cursor mode when their browser reports a fine pointer", () => {
	assert.equal(shouldUseRemoteTrackpad(true, "trackpad"), true);
	assert.equal(shouldUseRemoteTrackpad(true, "direct"), false);
	assert.equal(shouldUseRemoteTrackpad(false, "trackpad"), false);
});

test("phone portrait fullscreen keeps every Windows edge visible", () => {
	assert.equal(remoteFullscreenScaleMode(true, false), "fit");
	assert.equal(remoteFullscreenScaleMode(true, true), "fill");
	assert.equal(remoteFullscreenScaleMode(false, false), "fit");
	assert.equal(remoteFullscreenScaleMode(false, true), "fit");
});

test("active touch coordinates are rebased when mobile browser chrome changes the visual viewport", () => {
	const before = resolveRemoteViewport({
		innerWidth: 412,
		innerHeight: 915,
		visualViewport: { offsetLeft: 0, offsetTop: 72, width: 412, height: 780 },
	});
	const after = resolveRemoteViewport({
		innerWidth: 412,
		innerHeight: 915,
		visualViewport: { offsetLeft: 0, offsetTop: 0, width: 412, height: 852 },
	});
	assert.equal(shouldRebaseRemotePointerViewport(before, after, { width: 412, height: 780 }, { width: 412, height: 852 }, false), true);
	assert.equal(shouldRebaseRemotePointerViewport(after, after, { width: 412, height: 852 }, { width: 412, height: 852 }, false), false);
});

test("temporary IME geometry does not rebase a hidden remote pointer gesture", () => {
	const before = resolveRemoteViewport({ innerWidth: 412, innerHeight: 915, visualViewport: null });
	const ime = resolveRemoteViewport({
		innerWidth: 412,
		innerHeight: 915,
		visualViewport: { offsetLeft: 0, offsetTop: 0, width: 412, height: 336 },
	});
	assert.equal(shouldRebaseRemotePointerViewport(before, ime, { width: 412, height: 840 }, { width: 412, height: 310 }, true), false);
	assert.equal(shouldRebaseRemotePointerViewport(before, before, { width: 412, height: 840 }, { width: 390, height: 840 }, false), true);
});
