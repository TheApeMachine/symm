type Listener = () => void;

const createStore = <T>(initial: T) => {
	let value = initial;
	const listeners = new Set<Listener>();

	return {
		subscribe: (listener: Listener) => {
			listeners.add(listener);

			return () => {
				listeners.delete(listener);
			};
		},
		snapshot: () => value,
		set: (next: T) => {
			value = next;

			for (const listener of listeners) {
				listener();
			}
		},
	};
};

export const ConnectionStore = createStore(false);

export const useConnectionStore = () => ConnectionStore;
