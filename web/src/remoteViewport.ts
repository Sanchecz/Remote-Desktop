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

// Insets settle at different moments in Chromium/WebView, Firefox and Safari.
// These bounded passes replace open-ended resize loops and keep rotation
// deterministic even when the first event still reports portrait dimensions.
export const REMOTE_VIEWPORT_SETTLE_DELAYS = [0, 60, 180, 360, 700] as const;

export function remoteViewportChanged(previous: RemoteViewport, next: RemoteViewport): boolean {
	return previous.left !== next.left || previous.top !== next.top || previous.width !== next.width || previous.height !== next.height;
}
