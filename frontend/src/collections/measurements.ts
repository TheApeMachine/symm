import { createStore } from "@tanstack/react-store";

export const measurementsStore = createStore(
	{
		readings: {} as Record<string, Record<string, unknown>>,
	},
	({ setState }) => ({
		updateReading: (frame: Record<string, unknown>) => {
			setState((prev) => ({
				readings: {
					...prev.readings,
					[frame.origin as string]: {
						...prev.readings[frame.origin as string],
						[frame.scope as string]: frame,
					},
				},
			}));
		},
		reset: () => {
			setState((prev) => ({
				...prev,
				readings: {} as Record<string, Record<string, unknown>>,
			}));
		},
	}),
);
