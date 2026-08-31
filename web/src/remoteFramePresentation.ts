export type RemoteFrameRetirementScheduler = {
	retire: (url: string) => void;
	dispose: () => void;
};

type RemoteFrameRetirementOptions = {
	schedulePaint: (callback: FrameRequestCallback) => number;
	cancelPaint: (id: number) => void;
	revoke: (url: string) => void;
	paintCount?: number;
};

// A decoded JPEG may still be referenced by the browser compositor after the
// <img> element has accepted its successor. Revoking the old blob URL in the
// same task intermittently exposes an empty (black) GPU tile while Android is
// scaling the layer. Keep only the very small set of retired frames alive until
// the requested number of complete paints has passed, then release them.
export function createRemoteFrameRetirementScheduler(options: RemoteFrameRetirementOptions): RemoteFrameRetirementScheduler {
	const paintCount = Math.max(1, Math.round(options.paintCount || 2));
	const pending = new Map<string, number>();
	let paintRequest = 0;
	let disposed = false;

	const schedule = () => {
		if (disposed || paintRequest || pending.size === 0) return;
		paintRequest = options.schedulePaint(() => {
			paintRequest = 0;
			for (const [url, remaining] of pending) {
				if (remaining <= 1) {
					pending.delete(url);
					options.revoke(url);
				} else {
					pending.set(url, remaining - 1);
				}
			}
			schedule();
		});
	};

	return {
		retire(url: string) {
			if (!url || disposed) return;
			pending.set(url, Math.max(pending.get(url) || 0, paintCount));
			schedule();
		},
		dispose() {
			if (disposed) return;
			disposed = true;
			if (paintRequest) options.cancelPaint(paintRequest);
			paintRequest = 0;
			for (const url of pending.keys()) options.revoke(url);
			pending.clear();
		},
	};
}
