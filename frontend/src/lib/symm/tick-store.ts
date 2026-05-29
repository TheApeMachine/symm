type Listener = () => void;

let tickCount = 0;
let telemetryStatus = {
	seq: 0,
	throttled: false,
	queueDepth: 0,
	dropped: 0,
};
const listeners = new Set<Listener>();

export const TickStore = {
	subscribe(listener: Listener) {
		listeners.add(listener);

		return () => {
			listeners.delete(listener);
		};
	},

	snapshot() {
		return tickCount;
	},

	statusSnapshot() {
		return telemetryStatus;
	},

	ingest() {
		tickCount += 1;

		for (const listener of listeners) {
			listener();
		}
	},

	ingestHeartbeat(payload: {
		seq: number;
		throttled?: boolean;
		queue_depth?: number;
		dropped?: number;
	}) {
		tickCount = Math.max(tickCount, payload.seq);
		telemetryStatus = {
			seq: payload.seq,
			throttled: payload.throttled === true,
			queueDepth:
				typeof payload.queue_depth === "number" ? payload.queue_depth : 0,
			dropped: typeof payload.dropped === "number" ? payload.dropped : 0,
		};

		for (const listener of listeners) {
			listener();
		}
	},

	reset() {
		tickCount = 0;
		telemetryStatus = {
			seq: 0,
			throttled: false,
			queueDepth: 0,
			dropped: 0,
		};

		for (const listener of listeners) {
			listener();
		}
	},
};
