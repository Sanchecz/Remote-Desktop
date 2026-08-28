export type RemoteCamera = { zoom: number; panX: number; panY: number };
export type Point = { x: number; y: number };
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
