import { mergeFramePayload } from "#/providers/ws-frame-merge";

export const FRAME_INTERVAL_MS = 16;

/*
FrameBatcher coalesces websocket payloads before handing them to consumers.
Scheduling uses setTimeout because production batching runs in ws-worker.ts,
where requestAnimationFrame is unavailable. The 16ms window approximates 60Hz
but does not synchronize with display refresh; vsync-aligned delivery would
require requestAnimationFrame on the main thread when applying worker payloads.
*/
export class FrameBatcher {
	private queue: Record<string, unknown> = {};
	private nextTick: ReturnType<typeof setTimeout> | null = null;

	constructor(
		private readonly flush: (payload: Record<string, unknown>) => void,
		private readonly intervalMs = FRAME_INTERVAL_MS,
	) {}

	enqueue(incoming: Record<string, unknown>) {
		this.queue = mergeFramePayload(this.queue, incoming);

		if (this.nextTick !== null) {
			return;
		}

		this.nextTick = setTimeout(() => this.flushQueue(), this.intervalMs);
	}

	dispose() {
		if (this.nextTick !== null) {
			clearTimeout(this.nextTick);
			this.nextTick = null;
		}

		this.queue = {};
	}

	private flushQueue() {
		try {
			if (Object.keys(this.queue).length > 0) {
				this.flush(this.queue);
			}
		} finally {
			this.queue = {};
			this.nextTick = null;
		}
	}
}
