import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type Category = {
	type: string;
	confidence: number;
	surprisal: number;
	strength: number;
};

export type Measurement = {
	source: string;
	symbol: string;
	at: string;
	status: string;
	elapsed: number;
	entryBaseline: number;
	exitBaseline: number;
	categories: Category[];
	metrics: Record<string, number>;
};

/*
measurementsStore is the single source of truth for all signal data.
Every Measurement from the backend WebSocket is routed here, indexed
by source and by symbol.
*/
export const measurementsStore = createStore(
	{
		measurements: {} as Record<string, Record<string, CircularBuffer<Measurement>>>,
	},
	({ setState }) => ({
		updateFrame: (frames: Measurement[]) =>
			setState((prev) => {
				const measurements = { ...prev.measurements };

				for (const frame of frames) {
					if (!measurements[frame.symbol]) {
						measurements[frame.symbol] = {};
					}

					if (!measurements[frame.symbol][frame.source]) {
						measurements[frame.symbol][frame.source] = Circular<Measurement>(50);
					}

					measurements[frame.symbol][frame.source].push(frame);
				}

				return {
					measurements,
				};
			}),
	}),
);
