import { createStore } from "@tanstack/react-store";
import {
	dedupeEpoch,
	latestByMetric,
} from "#/components/terminal/measurement-view";
import type { Measurement } from "#/types/measurement";
import { Circular, type CircularBuffer } from "./circular";

export type { Category, Measurement } from "#/types/measurement";

/*
MeasurementEpoch is one backend observation batch for a symbol and source.
publishedAt records when the slot was written so kernel freshness can reflect
UI ingest time without re-copying the full measurement universe each tick.
*/
export type MeasurementEpoch = {
	at: string;
	readings: Measurement[];
	publishedAt: string;
};

type PendingEpoch = {
	symbol: string;
	source: string;
	readings: Measurement[];
};

export const measurementGroupKey = (symbol: string, source: string): string =>
	`${symbol}\u0000${source}`;

const pendingGroupKey = measurementGroupKey;

/*
epochAt chooses the observation timestamp shared by one measurement epoch.
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
headlineSeriesFromBuffer returns one headline sample per retained observation
epoch, oldest first, so sparklines preserve distinct market events delivered
together in one Thesis frame.
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
latestPublishedStamp reads the thesis-frame time of the newest retained slot.
*/
export const latestPublishedStamp = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): string | undefined => buffer?.values().at(-1)?.publishedAt;

/*
pushMeasurementEpoch appends one observation epoch without collapsing distinct
market events that arrived together in one Thesis frame.
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
		publishedAt: new Date().toISOString(),
	});
};

/*
measurementsStore retains backend measurements exactly as received. Buffers
mutate in place and version bumps notify subscribers without cloning the full
symbol universe on every websocket batch.
*/
export const measurementsStore = createStore(
	{
		measurements: {} as Record<
			string,
			Record<string, CircularBuffer<MeasurementEpoch>>
		>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frames: Measurement[]) =>
			setState((prev) => {
				if (frames.length === 0) {
					return prev;
				}

				const pending = new Map<string, PendingEpoch>();

				for (const frame of frames) {
					const groupKey = pendingGroupKey(frame.symbol, frame.source);
					const key = `${groupKey}\u0000${frame.at}`;
					const group = pending.get(key) ?? {
						symbol: frame.symbol,
						source: frame.source,
						readings: [],
					};

					group.readings.push(frame);
					pending.set(key, group);
				}

				const measurements = prev.measurements;

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
					version: prev.version + 1,
				};
			}),
	}),
);
