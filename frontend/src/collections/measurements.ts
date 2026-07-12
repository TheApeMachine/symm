import { createStore } from "@tanstack/react-store";
import type { Measurement } from "#/types/measurement";
import { Circular, type CircularBuffer } from "./circular";

export type { Category, Measurement } from "#/types/measurement";

/*
measurementEpochs groups the original records by their backend observation
time. A numerical signal emits one record per metric, so an epoch is the
smallest complete readout while every record remains unchanged in the store.
*/
export const measurementEpochs = (
	measurements: Measurement[],
): Measurement[][] => {
	const epochs = new Map<string, Measurement[]>();

	for (const measurement of measurements) {
		const epoch = epochs.get(measurement.at) ?? [];
		epoch.push(measurement);
		epochs.set(measurement.at, epoch);
	}

	return [...epochs.values()];
};

/*
measurementRaw reads one typed numerical value from an epoch. Metric and side
are both part of identity because directional Hawkes values share metric names.
*/
export const measurementRaw = (
	epoch: Measurement[],
	metric: string,
	side = "",
): number | null => {
	for (let index = epoch.length - 1; index >= 0; index -= 1) {
		const measurement = epoch[index];

		if (measurement.metric !== metric || (measurement.side ?? "") !== side) {
			continue;
		}

		return Number.isFinite(measurement.raw) ? measurement.raw : null;
	}

	return null;
};

/*
measurementsStore retains backend measurements exactly as received. Consumers
group typed records only when they need an epoch-level numerical readout.
*/
export const measurementsStore = createStore(
	{
		measurements: {} as Record<
			string,
			Record<string, CircularBuffer<Measurement>>
		>,
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
