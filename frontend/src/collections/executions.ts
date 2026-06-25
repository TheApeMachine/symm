import { createStore } from "@tanstack/react-store";

export const executionsStore = createStore(
	{
		frame: null as Record<string, unknown> | null,
		history: [] as Array<Record<string, unknown>>,
	},
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				const nextHistory = [...prev.history];
				for (const [key, value] of Object.entries(frame)) {
					if (
						key !== "role" &&
						key !== "scope" &&
						key !== "origin" &&
						key !== "destination" &&
						key !== "observed_at" &&
						key !== "timestamp_unix_nano" &&
						typeof value === "object" &&
						value !== null
					) {
						nextHistory.unshift({
							id: key,
							...(value as Record<string, unknown>),
						});
					}
				}

				if (nextHistory.length > 50) {
					nextHistory.length = 50;
				}

				return {
					frame,
					history: nextHistory,
				};
			}),
	}),
);

