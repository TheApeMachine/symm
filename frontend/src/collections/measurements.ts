import { createStore } from "@tanstack/react-store";

export interface MeasurementFrame {
	origin: string;
	scope: string;
	timestamp?: number;
	price?: number;
	output?: Record<string, unknown>;
	[key: string]: unknown;
}

export const measurementsStore = createStore(
	{
		readings: {} as Record<string, Record<string, MeasurementFrame>>,
	},
	({ setState }) => ({
		updateReading: (frame: Record<string, unknown>) => {
			setState((prev) => ({
				readings: {
					...prev.readings,
					[frame.origin as string]: {
						...prev.readings[frame.origin as string],
						[frame.scope as string]: frame as unknown as MeasurementFrame,
					},
				},
			}));
		},
		reset: () => {
			setState((prev) => ({
				...prev,
				readings: {} as Record<string, Record<string, MeasurementFrame>>,
			}));
		},
	}),
);

