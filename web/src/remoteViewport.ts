export type RemoteViewport = {
	left: number;
	top: number;
	width: number;
	height: number;
	landscape: boolean;
};

export type RemoteViewportInput = {
	innerWidth: number;
	innerHeight: number;
	documentWidth?: number;
	documentHeight?: number;
	visualViewport?: {
		offsetLeft: number;
		offsetTop: number;
		width: number;
		height: number;
	} | null;
};

export type RemoteLayoutOrientationInput = Pick<RemoteViewportInput, "innerWidth" | "innerHeight" | "documentWidth" | "documentHeight">;

const positive = (value: number | undefined, fallback: number) => Number.isFinite(value) && Number(value) > 0 ? Number(value) : fallback;
const coordinate = (value: number | undefined) => Number.isFinite(value) ? Math.max(0, Number(value)) : 0;

// Browser UI, display cut-outs and the Android keyboard change visualViewport
// independently from the CSS layout viewport. The visible rectangle is the
// only safe source for a fixed remote desktop after a portrait/landscape turn.
export function resolveRemoteViewport(input: RemoteViewportInput): RemoteViewport {
	const layoutWidth = positive(input.innerWidth, positive(input.documentWidth, 1));
	const layoutHeight = positive(input.innerHeight, positive(input.documentHeight, 1));
	const visual = input.visualViewport;
	const width = Math.max(1, Math.round(positive(visual?.width, layoutWidth)));
	const height = Math.max(1, Math.round(positive(visual?.height, layoutHeight)));
	return {
		left: Math.round(coordinate(visual?.offsetLeft)),
		top: Math.round(coordinate(visual?.offsetTop)),
		width,
		height,
		landscape: width > height,
	};
}

// A software keyboard normally changes only visualViewport. A real device
// rotation changes both the window layout viewport and the document viewport.
// Requiring these two independent layout measurements to agree lets browsers
// which omit/delay orientationchange still rotate the remote controller without
// mistaking an IME resize for a physical turn. `null` means the browser is in a
// transient two-phase layout and the previous physical orientation must win.
export function resolveRemoteLayoutLandscape(input: RemoteLayoutOrientationInput): boolean | null {
	const innerWidth = positive(input.innerWidth, 0);
	const innerHeight = positive(input.innerHeight, 0);
	if (innerWidth <= 0 || innerHeight <= 0) return null;
	const innerLandscape = innerWidth > innerHeight;
	const documentWidth = positive(input.documentWidth, 0);
	const documentHeight = positive(input.documentHeight, 0);
	if (documentWidth <= 0 || documentHeight <= 0) return innerLandscape;
	const documentLandscape = documentWidth > documentHeight;
	return innerLandscape === documentLandscape ? innerLandscape : null;
}

// Insets settle at different moments in Chromium/WebView, Firefox and Safari.
// These bounded passes replace open-ended resize loops and keep rotation
// deterministic even when the first event still reports portrait dimensions.
export const REMOTE_VIEWPORT_SETTLE_DELAYS = [0, 60, 180, 360, 700] as const;

export function remoteViewportChanged(previous: RemoteViewport, next: RemoteViewport): boolean {
	return previous.left !== next.left || previous.top !== next.top || previous.width !== next.width || previous.height !== next.height;
}

// PointerEvent.clientX/clientY are expressed in the browser's current visual
// viewport. Mobile browser chrome, safe-area insets and an orientation settle
// can replace that coordinate space while a finger is still down. Treat both
// visual-viewport and actual controller-surface changes as a gesture boundary;
// the caller can then rebase the next native sample instead of interpreting the
// viewport shift as a very large remote mouse movement.
export function shouldRebaseRemotePointerViewport(
	previous: RemoteViewport,
	next: RemoteViewport,
	previousSurface: { width: number; height: number },
	nextSurface: { width: number; height: number },
	keyboardOpen: boolean,
): boolean {
	if (keyboardOpen) return false;
	return remoteViewportChanged(previous, next)
		|| previousSurface.width !== nextSurface.width
		|| previousSurface.height !== nextSurface.height;
}

// Opening an on-screen keyboard can make a portrait visual viewport wider than
// it is tall even though the physical device did not rotate. Treat that resize
// as an IME transition, not an orientation change. A real orientationchange
// event passes `explicitOrientationChange=true`, closes the keyboard and
// rebuilds the fitted camera from the final viewport measurements.
export function shouldApplyRemoteOrientationTransition(
	previousLandscape: boolean,
	nextLandscape: boolean,
	keyboardOpen: boolean,
	explicitOrientationChange = false,
	layoutLandscape: boolean | null = null,
): boolean {
	if (explicitOrientationChange) return true;
	if (previousLandscape === nextLandscape) return false;
	if (!keyboardOpen) return true;
	return layoutLandscape === nextLandscape;
}

export function remoteViewportWithStableOrientation(
	viewport: RemoteViewport,
	physicalLandscape: boolean,
	keyboardOpen: boolean,
	layoutLandscape: boolean | null = null,
): RemoteViewport {
	if (!keyboardOpen || viewport.landscape === physicalLandscape) return viewport;
	if (layoutLandscape === viewport.landscape) return viewport;
	return { ...viewport, landscape: physicalLandscape };
}

// Some Android WebViews and mobile browsers expose a fine pointer even though
// the page is running on a phone. The visible viewport is therefore a second,
// deterministic signal for the compact remote controller. A real desktop
// keeps the full toolbar unless its window has deliberately been reduced to a
// phone/tablet-sized workspace, where the responsive controller is preferable.
export function shouldUseCompactRemoteControls(coarsePointer: boolean, viewport: Pick<RemoteViewport, "width" | "height">): boolean {
	return coarsePointer || (viewport.width <= 1024 && viewport.height <= 1024);
}

// The compact controller is the authoritative phone/tablet signal. In
// particular, iPadOS and several embedded Android browsers can expose a fine
// pointer even though the administrator is using a touchscreen. Basing the
// virtual-trackpad decision on the raw media query in that case makes the
// cursor disappear and turns relative movement into absolute taps after a
// rotation.
export function shouldUseRemoteTrackpad(compactRemoteClient: boolean, pointerMode: "direct" | "trackpad"): boolean {
	return compactRemoteClient && pointerMode === "trackpad";
}
