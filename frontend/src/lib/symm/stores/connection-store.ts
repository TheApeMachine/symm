import { createStore } from "@tanstack/react-store";

export type ConnectionState = {
	connected: boolean;
};

export const connectionStore = createStore<ConnectionState>({
	connected: false,
});

export const setFeedConnected = (connected: boolean): void => {
	connectionStore.setState((state) => {
		if (state.connected === connected) {
			return state;
		}

		return { connected };
	});
};
