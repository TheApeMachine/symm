import { createStore } from "@tanstack/react-store";

export type StreamName = "status" | "engine" | "field" | "chart";

export type ConnectionState = Record<StreamName, boolean>;

export const connectionStore = createStore<ConnectionState>({
	status: false,
	engine: false,
	field: false,
	chart: false,
});

export const setStreamConnected = (
	stream: StreamName,
	connected: boolean,
): void => {
	connectionStore.setState((state) => {
		if (state[stream] === connected) {
			return state;
		}

		return {
			...state,
			[stream]: connected,
		};
	});
};
