export type RemoteCamera = { zoom: number; panX: number; panY: number };
export type Point = { x: number; y: number };
export type Rect = { left: number; top: number; width: number; height: number };
export type RemoteTouchGestureMode = "pending" | "zoom" | "scroll";
export type RemotePointerSample = { x: number; y: number; time?: number };
export type RemoteTrackpadPendingMotion = Point | null;
export type RemotePointerTapAction = { action: "down" | "up"; button: "left" | "right" };

// A tap is one complete button transition. Sending duplicate downs before the
// release leaves some Windows applications in a drag/double-click state and
// makes a phone tap appear to land on the wrong control.
export function remotePointerTapActions(button: "left" | "right"): RemotePointerTapAction[] {
	return [
		{ action: "down", button },
		{ action: "up", button },
	];
}

// A newly mounted viewer can inherit the tail of the gesture which opened it,
// especially in Android WebView and mobile Safari. Never let that first contact
// arm a long-press/two-finger right click. One completed neutral gesture proves
// that subsequent contacts started inside the remote surface itself.
export function canStartRemoteRightClick(controlEnabled: boolean, frameReady: boolean, neutralGestureCompleted: boolean): boolean {
	return controlEnabled && frameReady && neutralGestureCompleted;
}

// Replacing an <img> source temporarily clears naturalWidth/naturalHeight in
// several mobile browsers. Keep using the last fully decoded frame while that
// happens; falling back straight to the rendered phone rectangle would move a
// 1080p/2K cursor into CSS-pixel space for one paint and then move it back.
export function authoritativeRemoteFrameSize(natural: Point, lastDecoded: Point, status: Point, rendered: Point): Point {
	// Width and height are one coordinate-space identity. During an image source
	// swap WebKit can expose only one new natural dimension for a paint. Picking
	// each axis independently would then manufacture a geometry which never
	// existed (for example 2560x1504 from a 2560x1440 frame and the previous
	// 2256x1504 frame). A pointer packet produced in that transient basis appears
	// to teleport when the complete image becomes available. Select the first
	// complete pair instead, keeping the previous decoded frame authoritative
	// until both dimensions of the new frame are known.
	for (const candidate of [natural, lastDecoded, status, rendered]) {
		if (Number.isFinite(candidate.x) && candidate.x > 0 && Number.isFinite(candidate.y) && candidate.y > 0) {
			return { x: Math.max(1, Math.round(candidate.x)), y: Math.max(1, Math.round(candidate.y)) };
		}
	}
	return { x: 1, y: 1 };
}

export function classifyRemoteTouchGesture(mode: RemoteTouchGestureMode, trackpad: boolean, scaleChange: number, midpointTravel: number): RemoteTouchGestureMode {
	if (!trackpad) return "zoom";
	// Lock the decision for the rest of the gesture. Finger spacing naturally
	// changes a little while performing a two-finger scroll; allowing a decided
	// scroll to turn into a pinch later caused the remote desktop to jump.
	if (mode !== "pending") return mode;
	if (Math.abs(scaleChange - 1) > 0.035) return "zoom";
	if (midpointTravel > 8) return "scroll";
	return mode;
}

export function isRemoteTwoFingerTap(mode: RemoteTouchGestureMode, trackpad: boolean, cancelled: boolean, elapsedMillis: number, midpointTravel: number): boolean {
	return trackpad && !cancelled && mode === "pending" && elapsedMillis < 420 && midpointTravel < 12;
}

// A pinch can finish between the event which schedules the camera update and
// the browser paint which applies the transformed image rectangle. Keep touch
// clicks suppressed until every finger is up and the pinch itself is inactive;
// the caller then waits for the committed paint before clearing suppression.
export function canReleaseRemoteTouchSuppression(activeTouchCount: number, pinchActive: boolean): boolean {
	return Number.isFinite(activeTouchCount) && activeTouchCount <= 0 && !pinchActive;
}

export function pointUnderScreenCoordinate(screen: Point, viewportCenter: Point, camera: RemoteCamera): Point {
	return {
		x: (screen.x - viewportCenter.x - camera.panX) / Math.max(0.0001, camera.zoom),
		y: (screen.y - viewportCenter.y - camera.panY) / Math.max(0.0001, camera.zoom),
	};
}

export function cameraKeepingPointUnderFingers(anchor: Point, midpoint: Point, viewportCenter: Point, zoom: number): RemoteCamera {
	return {
		zoom,
		panX: midpoint.x - viewportCenter.x - anchor.x * zoom,
		panY: midpoint.y - viewportCenter.y - anchor.y * zoom,
	};
}

// Advance a pinch from the last delivered touch sample, rather than repeatedly
// recalculating it from the gesture start. Mobile browsers may deliver several
// pointer moves inside one animation frame; accumulating each small movement
// prevents the image from snapping back towards the original midpoint.
export function advanceRemotePinch(camera: RemoteCamera, previousMidpoint: Point, midpoint: Point, viewportCenter: Point, previousDistance: number, distance: number): RemoteCamera {
	const anchor = pointUnderScreenCoordinate(previousMidpoint, viewportCenter, camera);
	const zoom = Math.max(1, Math.min(4, camera.zoom * Math.max(1, distance) / Math.max(1, previousDistance)));
	return cameraKeepingPointUnderFingers(anchor, midpoint, viewportCenter, zoom);
}

export function clampRemoteCamera(camera: RemoteCamera, content: Point, viewport: Point): RemoteCamera {
	const zoom = Math.max(1, Math.min(4, camera.zoom));
	// Keep the fitted desktop centred while it is smaller than the viewport. Once
	// it becomes larger, permit panning only until its edge reaches the matching
	// viewport edge. The previous "visible grip" boundary allowed almost the whole
	// frame to leave the phone display; the uncovered black canvas then appeared
	// as large rectangular flashes during pinch-in/pinch-out.
	const axisLimit = (contentLength: number, viewportLength: number) => {
		const scaled = Math.max(1, contentLength) * zoom;
		return Math.max(0, (scaled - Math.max(1, viewportLength)) / 2);
	};
	const maxPanX = axisLimit(content.x, viewport.x);
	const maxPanY = axisLimit(content.y, viewport.y);
	const clampAxis = (value: number, limit: number) => limit <= 0 ? 0 : Math.max(-limit, Math.min(limit, value));
	return {
		zoom,
		panX: clampAxis(camera.panX, maxPanX),
		panY: clampAxis(camera.panY, maxPanY),
	};
}

export function clampRemotePoint(point: Point, frame: Point): Point {
	return {
		x: Math.max(0, Math.min(Math.max(0, frame.x - 1), Math.round(point.x))),
		y: Math.max(0, Math.min(Math.max(0, frame.y - 1), Math.round(point.y))),
	};
}

// The Agent needs integer pixels, but the local mobile cursor is rendered in
// the browser and must retain sub-pixel progress. Quantising every touch sample
// made slow movements on fitted 2K/4K desktops alternate between stopping and
// jumping. Keep a stable compositor coordinate and round only the packet sent
// through clampRemotePoint(). Three decimal places are well below a display
// pixel at every supported zoom while avoiding noisy CSS strings.
export function remoteCursorVisualPoint(point: Point, frame: Point): Point {
	const clampAxis = (value: number, length: number) => {
		const maximum = Math.max(0, (Number.isFinite(length) ? length : 1) - 1);
		const finite = Number.isFinite(value) ? value : 0;
		return Math.round(Math.max(0, Math.min(maximum, finite)) * 1000) / 1000;
	};
	return {
		x: clampAxis(point.x, frame.x),
		y: clampAxis(point.y, frame.y),
	};
}

// The decoded JPEG can switch geometry (for example, a sharp 4K rest frame to
// a lighter 2K interaction frame) one browser paint before React commits the
// matching image-layer dimensions. Keep the local cursor in the coordinate
// system of the layer that is actually on screen during that short boundary.
// Without this projection a centred 2K cursor was drawn at x=1280 inside a
// still-3840px layer and visibly jumped left until the following React commit.
export function remoteCursorVisualPointForLayer(point: Point, coordinateFrame: Point, layerFrame: Point): Point {
	const source = {
		x: Math.max(1, Number.isFinite(coordinateFrame.x) ? coordinateFrame.x : 1),
		y: Math.max(1, Number.isFinite(coordinateFrame.y) ? coordinateFrame.y : 1),
	};
	const target = layerFrame.x > 0 && layerFrame.y > 0 && Number.isFinite(layerFrame.x) && Number.isFinite(layerFrame.y)
		? layerFrame
		: source;
	const projected = source.x === target.x && source.y === target.y
		? point
		: reprojectRemotePoint(point, source, target);
	return remoteCursorVisualPoint(projected, target);
}

// A decode already in progress must be presented even when a newer compressed
// frame arrived meanwhile. Comparing it with the latest *arrival* can starve a
// 60 FPS stream forever whenever JPEG decoding takes longer than one producer
// interval. Compare only with the last frame actually presented; the one-slot
// pending queue will then immediately decode the newest waiting frame.
export function shouldPresentDecodedRemoteFrame(candidateOrder: number, lastPresentedOrder: number): boolean {
	return Number.isSafeInteger(candidateOrder)
		&& Number.isSafeInteger(lastPresentedOrder)
		&& candidateOrder > lastPresentedOrder;
}

// JPEG dimensions may change while a session stays open: the Agent uses a
// sharper idle profile and a lower-latency interaction profile, and Windows can
// also change monitor geometry after RDP, docking or rotation. Keep the remote
// cursor on the same normalized desktop point instead of interpreting its old
// pixel coordinates in the new frame (which looked like a teleport on phones).
export function reprojectRemotePoint(point: Point, fromFrame: Point, toFrame: Point): Point {
	const fromWidth = Math.max(1, fromFrame.x);
	const fromHeight = Math.max(1, fromFrame.y);
	const toWidth = Math.max(1, toFrame.x);
	const toHeight = Math.max(1, toFrame.y);
	// Pixel coordinates have inclusive endpoints (0..width-1). Mapping by raw
	// widths makes the far edge lose a fraction on every idle/interactive frame
	// transition, so a cursor near an edge slowly drifts after repeated profile
	// changes. Preserve the normalized inclusive pixel position instead.
	const projectAxis = (value: number, fromLength: number, toLength: number) => {
		if (toLength <= 1) return 0;
		if (fromLength <= 1) return Math.max(0, Math.min(toLength - 1, value));
		return Math.max(0, Math.min(toLength - 1, value * (toLength - 1) / (fromLength - 1)));
	};
	return {
		x: projectAxis(point.x, fromWidth, toWidth),
		y: projectAxis(point.y, fromHeight, toHeight),
	};
}

// A phone touchpad receives fractional CSS-pixel deltas. Rounding after every
// event discards those fractions and makes the remote cursor alternately stop
// and jump, especially when a 1080p/2K/4K desktop is fitted into a phone. Keep
// the precise remote position between events and round only the packet sent to
// the Agent. A gentle continuous acceleration gives slow movements pixel-level
// precision while keeping long swipes practical on a small screen.
export function advanceRemoteTrackpadCursor(current: Point, delta: Point, frame: Point, rendered: Point): Point {
	const frameWidth = Math.max(1, frame.x);
	const frameHeight = Math.max(1, frame.y);
	const renderedWidth = Math.max(1, rendered.x);
	const renderedHeight = Math.max(1, rendered.y);
	const travel = Math.hypot(delta.x, delta.y);
	const acceleration = Math.min(1.5, 0.55 + Math.sqrt(Math.max(0, travel)) * 0.17);
	return {
		x: Math.max(0, Math.min(frameWidth - 1, current.x + delta.x * frameWidth / renderedWidth * acceleration)),
		y: Math.max(0, Math.min(frameHeight - 1, current.y + delta.y * frameHeight / renderedHeight * acceleration)),
	};
}

// Pointer capture in mobile Chromium/WebView can emit one sample in the old
// visual-viewport coordinate system after browser chrome or orientation moves.
// Treat an impossible jump as a rebase and softly bound merely coalesced fast
// samples. This prevents a single browser glitch from sending the remote cursor
// to an edge while preserving natural acceleration for real swipes.
export function stableRemoteTrackpadDelta(delta: Point, rendered: Point): Point {
	const x = Number.isFinite(delta.x) ? delta.x : 0;
	const y = Number.isFinite(delta.y) ? delta.y : 0;
	const travel = Math.hypot(x, y);
	if (travel === 0) return { x: 0, y: 0 };
	// Use the shorter controller dimension. Browser chrome makes portrait and
	// landscape viewports very asymmetric; basing the cap on the long edge let a
	// solitary inset sample move a 2256x1504 cursor by several hundred remote
	// pixels before the next event pulled it back.
	const renderedExtent = Math.max(1, Math.min(rendered.x, rendered.y));
	const impossibleTravel = Math.max(120, renderedExtent * 0.35);
	if (travel > impossibleTravel) return { x: 0, y: 0 };
	// A genuine fast swipe is normally represented by several coalesced pointer
	// samples (see stableRemoteTrackpadSamples below). A single 80-100 CSS-pixel
	// jump is therefore much more likely to be a visualViewport/pointer-capture
	// rebase than useful motion. Keep the emergency cap deliberately below one
	// tenth of a phone landscape canvas so one bad sample cannot throw a 2K/4K
	// cursor hundreds of remote pixels away.
	const boundedTravel = Math.max(28, Math.min(52, renderedExtent * 0.07));
	if (travel <= boundedTravel) return { x, y };
	const scale = boundedTravel / travel;
	return { x: x * scale, y: y * scale };
}

// Chromium, WebView and Safari can batch several physical touch samples into
// one pointermove. Filtering only the final event either discards a legitimate
// fast swipe or accepts it as one large teleport. Preserve every coalesced
// sample, reject only a coordinate-space rebase that is proven by a following
// settled sample, and let the caller render/send the newest resulting cursor
// once per browser event. A solitary fast sample is deliberately retained:
// stabilizeRemoteTrackpadMotion can compare it with the next browser event and
// distinguish real continued motion from the characteristic jump-return pair.
export function stableRemoteTrackpadSamples(previous: Point, samples: RemotePointerSample[], rendered: Point, previousTime = 0): { deltas: Point[]; last: Point; lastTime: number } {
	let last = {
		x: Number.isFinite(previous.x) ? previous.x : 0,
		y: Number.isFinite(previous.y) ? previous.y : 0,
	};
	let lastTime = Number.isFinite(previousTime) ? previousTime : 0;
	const deltas: Point[] = [];
	const shortExtent = Math.max(1, Math.min(rendered.x, rendered.y));
	// A viewport rebase has a characteristic shape: the first coalesced sample
	// suddenly moves tens of CSS pixels and the following sample resumes with a
	// small, ordinary delta in the new coordinate space. This is common when the
	// Android browser bar settles or a WebView changes its display inset. Merely
	// capping that first delta still throws a 2256x1504/4K cursor by hundreds of
	// remote pixels. Some engines expose that rebase as the only sample in the
	// event, so an isolated first jump above the same threshold is also absorbed.
	// A real fast swipe normally contributes continued/coalesced samples and is
	// fully preserved; at worst an ambiguous single event pauses motion once
	// instead of teleporting a 2K/4K cursor and then snapping it back.
	const rebaseTravel = Math.max(42, Math.min(84, shortExtent * 0.12));
	const settledTravel = Math.max(14, Math.min(34, shortExtent * 0.08));
	for (let index = 0; index < samples.length; index += 1) {
		const sample = samples[index];
		if (!Number.isFinite(sample.x) || !Number.isFinite(sample.y)) continue;
		const next = { x: sample.x, y: sample.y };
		const nextTime = Number.isFinite(sample.time) ? Number(sample.time) : lastTime;
		const rawDelta = { x: next.x - last.x, y: next.y - last.y };
		const following = samples[index + 1];
		const followingTravel = following && Number.isFinite(following.x) && Number.isFinite(following.y)
			? Math.hypot(following.x - next.x, following.y - next.y)
			: Number.POSITIVE_INFINITY;
		const firstSampleTravel = Math.hypot(rawDelta.x, rawDelta.y);
		if (index === 0 && firstSampleTravel > rebaseTravel && followingTravel <= settledTravel) {
			last = next;
			lastTime = nextTime;
			continue;
		}
		const delta = stableRemoteTrackpadDelta(rawDelta, rendered);
		last = next;
		lastTime = nextTime;
		if (delta.x !== 0 || delta.y !== 0) deltas.push(delta);
	}
	return { deltas, last, lastTime };
}

// A few Android browser/WebView combinations occasionally deliver a pair of
// individually plausible pointer samples in different visual-viewport bases:
// one quick jump followed one event later by an almost equal jump back. Each
// sample is below the hard rebase threshold above, so applying the first one
// makes the local cursor visibly teleport and then return. Hold only a rapid,
// solitary, unusually large delta for one short event. A matching reversal is
// discarded; sustained motion is released in order with at most one-event lag.
// Ordinary movement and coalesced samples stay on the zero-look-ahead path.
export function stabilizeRemoteTrackpadMotion(
	pending: RemoteTrackpadPendingMotion,
	deltas: Point[],
	rendered: Point,
	elapsedMillis: number,
): { deltas: Point[]; pending: RemoteTrackpadPendingMotion } {
	const output: Point[] = [];
	let remaining = deltas.filter((delta) => Number.isFinite(delta.x) && Number.isFinite(delta.y) && (delta.x !== 0 || delta.y !== 0));
	const shortExtent = Math.max(1, Math.min(rendered.x, rendered.y));
	const ambiguousTravel = Math.max(24, Math.min(38, shortExtent * 0.06));
	const rapid = !Number.isFinite(elapsedMillis) || elapsedMillis <= 0 || elapsedMillis < 48;

	if (pending) {
		if (!remaining.length) return { deltas: output, pending };
		const current = remaining[0];
		const pendingTravel = Math.hypot(pending.x, pending.y);
		const currentTravel = Math.hypot(current.x, current.y);
		const direction = pendingTravel > 0 && currentTravel > 0
			? (pending.x * current.x + pending.y * current.y) / (pendingTravel * currentTravel)
			: 1;
		const residual = Math.hypot(pending.x + current.x, pending.y + current.y);
		const reversal = rapid
			&& direction < -0.72
			&& residual <= Math.max(10, Math.min(pendingTravel, currentTravel) * 0.55);
		if (reversal) remaining = remaining.slice(1);
		else output.push(pending);
	}

	if (remaining.length === 1) {
		const candidate = remaining[0];
		if (rapid && Math.hypot(candidate.x, candidate.y) >= ambiguousTravel) {
			return { deltas: output, pending: candidate };
		}
	}
	output.push(...remaining);
	return { deltas: output, pending: null };
}

// Fit the complete remote desktop inside the available canvas without ever
// rounding one dimension beyond the viewport. This is the default camera in
// both phone orientations and remains correct for 1080p, 2K and 4K sources.
export function fitRemoteFrame(frame: Point, viewport: Point): Point {
	if (frame.x <= 0 || frame.y <= 0 || viewport.x <= 0 || viewport.y <= 0) return { x: 1, y: 1 };
	const ratio = Math.min(viewport.x / frame.x, viewport.y / frame.y);
	return {
		x: Math.max(1, Math.min(viewport.x, Math.floor(frame.x * ratio))),
		y: Math.max(1, Math.min(viewport.y, Math.floor(frame.y * ratio))),
	};
}

// Fill every canvas pixel while preserving the remote desktop aspect ratio.
// The mismatched axis deliberately extends past the viewport and is handled by
// the same bounded camera used for pinch/pan. This is the useful phone portrait
// alternative to letterboxing: no black bands, with a predictable edge crop.
export function fillRemoteFrame(frame: Point, viewport: Point): Point {
	if (frame.x <= 0 || frame.y <= 0 || viewport.x <= 0 || viewport.y <= 0) return { x: 1, y: 1 };
	const ratio = Math.max(viewport.x / frame.x, viewport.y / frame.y);
	return {
		x: Math.max(viewport.x, Math.ceil(frame.x * ratio)),
		y: Math.max(viewport.y, Math.ceil(frame.y * ratio)),
	};
}

// The transformed image rectangle is authoritative for pointer mapping. This
// remains exact after fit, fixed scale, pinch zoom and panning. Coordinates in
// the black letterbox are intentionally clamped to the nearest desktop edge so
// a phone trackpad never develops a dead area around the remote monitor.
export function remotePointFromClient(client: Point, imageRect: Rect, frame: Point): Point {
	const width = Math.max(1, imageRect.width);
	const height = Math.max(1, imageRect.height);
	return clampRemotePoint({
		x: (client.x - imageRect.left) * Math.max(1, frame.x) / width,
		y: (client.y - imageRect.top) * Math.max(1, frame.y) / height,
	}, frame);
}

// In mobile cursor mode the pointer is independent from the finger. If it
// reaches the visible viewport edge while zoomed, pan only as far as required
// to keep the cursor on screen. This mirrors native Remote Desktop clients and
// does not disturb the cursor's remote coordinates.
export function cameraFollowingRemotePoint(camera: RemoteCamera, remote: Point, frame: Point, fitted: Point, viewport: Point, margin = 42): RemoteCamera {
	if (camera.zoom <= 1.0001 || frame.x <= 0 || frame.y <= 0 || fitted.x <= 0 || fitted.y <= 0 || viewport.x <= 0 || viewport.y <= 0) {
		return clampRemoteCamera(camera, fitted, viewport);
	}
	const safeMarginX = Math.min(Math.max(12, margin), viewport.x / 3);
	const safeMarginY = Math.min(Math.max(12, margin), viewport.y / 3);
	const scaleX = fitted.x * camera.zoom / frame.x;
	const scaleY = fitted.y * camera.zoom / frame.y;
	const screenX = viewport.x / 2 + camera.panX + (remote.x - frame.x / 2) * scaleX;
	const screenY = viewport.y / 2 + camera.panY + (remote.y - frame.y / 2) * scaleY;
	let panX = camera.panX;
	let panY = camera.panY;
	if (screenX < safeMarginX) panX += safeMarginX - screenX;
	else if (screenX > viewport.x - safeMarginX) panX -= screenX - (viewport.x - safeMarginX);
	if (screenY < safeMarginY) panY += safeMarginY - screenY;
	else if (screenY > viewport.y - safeMarginY) panY -= screenY - (viewport.y - safeMarginY);
	return clampRemoteCamera({ zoom: camera.zoom, panX, panY }, fitted, viewport);
}
