import { mergeFramePayload } from "#/providers/ws-frame-merge";

export const FRAME_INTERVAL_MS = 16;

/*
FrameBatcher coalesces websocket payloads on a 60Hz window before handing them
to the main thread, matching display refresh without dropping the newest frame.
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
		if (Object.keys(this.queue).length > 0) {
			this.flush(this.queue);
			this.queue = {};
		}

		this.nextTick = null;
	}
}
