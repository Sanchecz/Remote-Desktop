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
	// A fitted desktop is commonly letterboxed on a portrait phone. The old
	// bounds collapsed that empty axis to `overscan`, so an otherwise correct
	// off-centre pinch was immediately pulled back towards the viewport centre.
	// The absolute half-difference lets the fitted frame travel through its
	// letterbox as it grows, while retaining the same finite boundary once the
	// zoomed frame becomes larger than the viewport.
	const maxPanX = Math.abs(Math.max(1, content.x) * zoom - Math.max(1, viewport.x)) / 2 + overscan;
	const maxPanY = Math.abs(Math.max(1, content.y) * zoom - Math.max(1, viewport.y)) / 2 + overscan;
	return {
		zoom,
		panX: Math.max(-maxPanX, Math.min(maxPanX, camera.panX)),
		panY: Math.max(-maxPanY, Math.min(maxPanY, camera.panY)),
	};
}
