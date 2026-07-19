import type { CircularBuffer } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/types";
import { seriesByMetric } from "#/components/terminal/measurement-view";

/*
flattenMeasurementBuffer returns retained measurements from a leaf buffer or
passes through an already-flat array delivered by the worker paint channel.
*/
export const flattenMeasurementBuffer = (
	buffer: CircularBuffer<Measurement> | Measurement[] | undefined,
): Measurement[] => {
	if (buffer === undefined) {
		return [];
	}

	if (Array.isArray(buffer)) {
		return buffer;
	}

	return buffer.values();
};

/*
measurementTickCount reports how many observation timestamps a flat buffer
retains. Flat Measurement rows share an `at` stamp within one backend epoch.
*/
export const measurementTickCount = (
	buffer: CircularBuffer<Measurement> | Measurement[] | undefined,
): number => {
	const rows = flattenMeasurementBuffer(buffer);

	if (rows.length === 0) {
		return 0;
	}

	return new Set(rows.map((row) => row.at)).size;
};

/*
latestMeasurementReadings returns every measurement that shares the newest
observation timestamp in a flat history.
*/
export const latestMeasurementReadings = (
	buffer: CircularBuffer<Measurement> | Measurement[] | undefined,
): Measurement[] => {
	const rows = flattenMeasurementBuffer(buffer);
	const at = rows.at(-1)?.at;

	if (at === undefined) {
		return [];
	}

	return rows.filter((row) => row.at === at);
};

/*
headlineSeriesFromBuffer returns one headline sample per retained observation
epoch so sparklines preserve distinct market events.
*/
export const headlineSeriesFromBuffer = (
	buffer: CircularBuffer<Measurement> | Measurement[] | undefined,
	headline: string,
	side = "",
): number[] => seriesByMetric(flattenMeasurementBuffer(buffer), headline, side);

/*
latestPublishedStamp reads the observation time of the newest retained row.
*/
export const latestPublishedStamp = (
	buffer: CircularBuffer<Measurement> | Measurement[] | undefined,
): string | undefined => flattenMeasurementBuffer(buffer).at(-1)?.at;

/*
measurementEpochs groups flat records by backend observation time.
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
measurementRaw reads one typed numerical value from an epoch.
*/
export const measurementRaw = (
	epoch: Measurement[],
	metric: string,
	side = "",
): number | null => {
	const key = side === "" ? metric : `${metric}:${side}`;

	for (let index = epoch.length - 1; index >= 0; index -= 1) {
		const measurement = epoch[index];

		if (
			measurement.metric === metric &&
			(measurement.side ?? "") === side &&
			Number.isFinite(measurement.raw)
		) {
			return measurement.raw;
		}

		const raw = measurement.metrics?.[key];

		if (typeof raw === "number" && Number.isFinite(raw)) {
			return raw;
		}
	}

	return null;
};

/*
epochsFromMeasurements rebuilds MeasurementEpoch slots for Hawkes fit helpers
that still join multi-metric frames by observation time.
*/
export const epochsFromMeasurements = (
	frames: Measurement[],
): MeasurementEpoch[] =>
	measurementEpochs(frames).map((readings) => ({
		at: readings[0]?.at ?? "",
		readings,
		publishedAt: readings.at(-1)?.at ?? readings[0]?.at ?? "",
	}));

/*
measurementsForSource keeps rows that belong to one signal source.
*/
export const measurementsForSource = (
	frames: Measurement[],
	source: string,
): Measurement[] => frames.filter((row) => row.source === source);

/*
measurementsForSymbol keeps rows for one traded pair.
*/
export const measurementsForSymbol = (
	frames: Measurement[],
	symbol: string,
): Measurement[] => frames.filter((row) => row.symbol === symbol);
