export type RemoteCamera = { zoom: number; panX: number; panY: number };
export type Point = { x: number; y: number };

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

export function clampRemoteCamera(camera: RemoteCamera, content: Point, viewport: Point, overscan = 32): RemoteCamera {
	const zoom = Math.max(1, Math.min(4, camera.zoom));
	const maxPanX = Math.max(0, (Math.max(1, content.x) * zoom - Math.max(1, viewport.x)) / 2) + overscan;
	const maxPanY = Math.max(0, (Math.max(1, content.y) * zoom - Math.max(1, viewport.y)) / 2) + overscan;
	return {
		zoom,
		panX: Math.max(-maxPanX, Math.min(maxPanX, camera.panX)),
		panY: Math.max(-maxPanY, Math.min(maxPanY, camera.panY)),
	};
}
