export type RemoteCamera = { zoom: number; panX: number; panY: number };
export type Point = { x: number; y: number };
export type Rect = { left: number; top: number; width: number; height: number };
export type RemoteTouchGestureMode = "pending" | "zoom" | "scroll";

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

export function clampRemoteCamera(camera: RemoteCamera, content: Point, viewport: Point, overscan = 32): RemoteCamera {
	const zoom = Math.max(1, Math.min(4, camera.zoom));
	// At 100% the fitted desktop stays centred. Once the administrator zooms in,
	// allow any chosen edge (including the bottom of a letterboxed portrait
	// viewport) to travel beneath the fingers while keeping a small, recoverable
	// piece of the frame visible. The previous half-difference clamp pulled a
	// valid bottom-edge pinch back towards the viewport centre.
	const axisLimit = (contentLength: number, viewportLength: number) => {
		if (zoom <= 1.0001) return 0;
		const scaled = Math.max(1, contentLength) * zoom;
		const visibleGrip = Math.max(48, Math.min(96, scaled / 3, Math.max(1, viewportLength) / 3));
		return Math.max(0, (scaled + Math.max(1, viewportLength)) / 2 - visibleGrip) + overscan;
	};
	const maxPanX = axisLimit(content.x, viewport.x);
	const maxPanY = axisLimit(content.y, viewport.y);
	return {
		zoom,
		panX: Math.max(-maxPanX, Math.min(maxPanX, camera.panX)),
		panY: Math.max(-maxPanY, Math.min(maxPanY, camera.panY)),
	};
}

export function clampRemotePoint(point: Point, frame: Point): Point {
	return {
		x: Math.max(0, Math.min(Math.max(0, frame.x - 1), Math.round(point.x))),
		y: Math.max(0, Math.min(Math.max(0, frame.y - 1), Math.round(point.y))),
	};
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
