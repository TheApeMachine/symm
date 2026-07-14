import { createStore } from "@tanstack/react-store";
import {
	dedupeEpoch,
	latestByMetric,
} from "#/components/terminal/measurement-view";
import type { Measurement } from "#/types/measurement";
import { Circular, type CircularBuffer } from "./circular";

export type { Category, Measurement } from "#/types/measurement";

/*
MeasurementEpoch is one backend observation tick: every metric emitted together
at the same timestamp for one symbol and source.
*/
export type MeasurementEpoch = {
	at: string;
	readings: Measurement[];
};

type PendingEpoch = {
	symbol: string;
	source: string;
	readings: Measurement[];
};

const pendingGroupKey = (symbol: string, source: string): string =>
	`${symbol}\u0000${source}`;

/*
epochAt chooses the newest observation timestamp present in one publish batch.
*/
const epochAt = (readings: Measurement[]): string => {
	let latest = readings[0]?.at ?? "";

	for (const reading of readings) {
		if (reading.at > latest) {
			latest = reading.at;
		}
	}

	return latest;
};

/*
headlineSeriesFromBuffer returns one headline sample per retained publish tick,
oldest first, so sparklines advance on every thesis frame rather than market
observation timestamps that can remain unchanged between ticks.
*/
export const headlineSeriesFromBuffer = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
	headline: string,
	side = "",
): number[] => {
	if (buffer === undefined) {
		return [];
	}

	return buffer.values().flatMap((epoch) => {
		const measurement = latestByMetric(epoch.readings, headline, side);

		return measurement !== undefined && Number.isFinite(measurement.raw)
			? [measurement.raw]
			: [];
	});
};

/*
flattenMeasurementBuffer expands retained observation ticks into the flat
measurement list sparklines and epoch helpers expect.
*/
export const flattenMeasurementBuffer = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): Measurement[] => {
	if (buffer === undefined) {
		return [];
	}

	return buffer.values().flatMap((epoch) => epoch.readings);
};

/*
latestMeasurementReadings returns the newest complete observation tick.
*/
export const latestMeasurementReadings = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): Measurement[] => buffer?.values().at(-1)?.readings ?? [];

/*
measurementTickCount reports how many observation ticks the buffer retains.
*/
export const measurementTickCount = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): number => buffer?.length() ?? 0;

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
pushMeasurementEpoch appends one publish-batch slot so the circular buffer
advances on every thesis tick even when market observation timestamps repeat.
*/
const pushMeasurementEpoch = (
	buffer: CircularBuffer<MeasurementEpoch>,
	readings: Measurement[],
): void => {
	if (readings.length === 0) {
		return;
	}

	buffer.push({
		at: epochAt(readings),
		readings: dedupeEpoch(readings),
	});
};

/*
measurementsStore retains backend measurements exactly as received. Each
circular-buffer slot is one observation tick, not one metric record.
*/
export const measurementsStore = createStore(
	{
		measurements: {} as Record<
			string,
			Record<string, CircularBuffer<MeasurementEpoch>>
		>,
	},
	({ setState }) => ({
		updateFrame: (frames: Measurement[]) =>
			setState((prev) => {
				const measurements = { ...prev.measurements };
				const pending = new Map<string, PendingEpoch>();

				for (const frame of frames) {
					const key = pendingGroupKey(frame.symbol, frame.source);
					const group = pending.get(key) ?? {
						symbol: frame.symbol,
						source: frame.source,
						readings: [],
					};

					group.readings.push(frame);
					pending.set(key, group);
				}

				for (const group of pending.values()) {
					if (!measurements[group.symbol]) {
						measurements[group.symbol] = {};
					}

					if (!measurements[group.symbol][group.source]) {
						measurements[group.symbol][group.source] =
							Circular<MeasurementEpoch>(50);
					}

					pushMeasurementEpoch(
						measurements[group.symbol][group.source],
						group.readings,
					);
				}

				return {
					measurements,
				};
			}),
	}),
);
