type Listener = () => void;

let tickCount = 0;
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

	ingest() {
		tickCount += 1;

		for (const listener of listeners) {
			listener();
		}
	},

	reset() {
		tickCount = 0;

		for (const listener of listeners) {
			listener();
		}
	},
};
