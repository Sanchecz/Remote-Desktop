import assert from "node:assert/strict";
import test from "node:test";
import { REMOTE_VIEWPORT_SETTLE_DELAYS, remoteViewportChanged, resolveRemoteViewport } from "../src/remoteViewport.ts";

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

test("invalid embedded viewport measurements never collapse the session", () => {
	assert.deepEqual(resolveRemoteViewport({
		innerWidth: 0,
		innerHeight: Number.NaN,
		documentWidth: 360,
		documentHeight: 640,
		visualViewport: { offsetLeft: -4, offsetTop: -2, width: 0, height: Number.NaN },
	}), { left: 0, top: 0, width: 360, height: 640, landscape: false });
});
