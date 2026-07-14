import type { CircularBuffer } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/measurements";

/*
HawkesCurveSegment is one identifiable interval between consecutive arrivals.
It retains the fitted kernel values required to draw the actual decay curve.
*/
export type HawkesCurveSegment = {
	fromAt: number;
	throughAt: number;
	beforeArrival: number;
	afterArrival: number;
	throughIntensity: number;
	baseline: number;
	beta: number;
};

/*
HawkesModel is the fitted univariate process shared by one parameter epoch.
*/
type HawkesModel = {
	baseline: number;
	beta: number;
};

/*
HawkesObservation is the pre-arrival intensity emitted for one market event.
*/
type HawkesObservation = {
	at: number;
	intensity: number;
	fit: string;
};

/*
reading selects one directional metric from a complete observation epoch.
*/
const reading = (
	epoch: MeasurementEpoch,
	metric: string,
	side = "",
): Measurement | undefined => {
	for (let index = epoch.readings.length - 1; index >= 0; index -= 1) {
		const measurement = epoch.readings[index];

		if (measurement.metric === metric && (measurement.side ?? "") === side) {
			return measurement;
		}
	}

	return undefined;
};

/*
raw returns a finite metric value without replacing absent model data.
*/
const raw = (
	epoch: MeasurementEpoch,
	metric: string,
	side = "",
): number | null => {
	const value = reading(epoch, metric, side)?.raw;

	return Number.isFinite(value) ? (value as number) : null;
};

/*
fitKey identifies the parameter interval that produced a measurement.
*/
const fitKey = (measurement: Measurement | undefined): string => {
	const from = measurement?.scale?.from ?? "";
	const through = measurement?.scale?.through ?? "";

	return from === "" || through === "" ? "" : `${from}\u0000${through}`;
};

/*
modelFrom reads one complete fitted kernel from an emitted model epoch.
*/
const modelFrom = (epoch: MeasurementEpoch): [string, HawkesModel] | null => {
	const buy = reading(epoch, "baseline_intensity", "buy");
	const sell = reading(epoch, "baseline_intensity", "sell");
	const beta = raw(epoch, "decay_rate");
	const buyFit = fitKey(buy);
	const sellFit = fitKey(sell);

	if (
		buy === undefined ||
		sell === undefined ||
		!Number.isFinite(buy.raw) ||
		!Number.isFinite(sell.raw) ||
		beta === null ||
		beta <= 0 ||
		buyFit === "" ||
		buyFit !== sellFit
	) {
		return null;
	}

	return [buyFit, { baseline: buy.raw + sell.raw, beta }];
};

/*
observationFrom reads the total pre-arrival intensity for one market event.
*/
const observationFrom = (epoch: MeasurementEpoch): HawkesObservation | null => {
	const buy = reading(epoch, "conditional_intensity", "buy");
	const sell = reading(epoch, "conditional_intensity", "sell");
	const at = Date.parse(epoch.at);
	const buyFit = fitKey(buy);
	const sellFit = fitKey(sell);

	if (
		buy === undefined ||
		sell === undefined ||
		!Number.isFinite(buy.raw) ||
		!Number.isFinite(sell.raw) ||
		!Number.isFinite(at) ||
		buyFit === "" ||
		buyFit !== sellFit
	) {
		return null;
	}

	return { at, intensity: buy.raw + sell.raw, fit: buyFit };
};

/*
latestHawkesRaw carries fitted parameters forward within the active fit epoch.
Matching scale identity prevents a missing current parameter from silently
falling back to a stale model.
*/
export const latestHawkesRaw = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
	metric: string,
	side = "",
): number | null => {
	const epochs = buffer?.values() ?? [];
	let currentFit = "";

	for (let index = epochs.length - 1; index >= 0; index -= 1) {
		const observation = observationFrom(epochs[index]);

		if (observation !== null) {
			currentFit = observation.fit;
			break;
		}
	}

	if (currentFit === "") {
		return null;
	}

	for (let index = epochs.length - 1; index >= 0; index -= 1) {
		const measurement = reading(epochs[index], metric, side);
		const value = measurement?.raw;

		if (
			value !== undefined &&
			Number.isFinite(value) &&
			fitKey(measurement) === currentFit
		) {
			return value;
		}
	}

	return null;
};

/*
hawkesCurveFromBuffer reconstructs the identifiable intensity path for the
current fitted parameter epoch. Consecutive pre-arrival intensities determine
the preceding post-arrival jump exactly under the shared exponential kernel.
*/
export const hawkesCurveFromBuffer = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): HawkesCurveSegment[] => {
	const epochs = buffer?.values() ?? [];
	const models = new Map<string, HawkesModel>();
	const observations: HawkesObservation[] = [];

	for (const epoch of epochs) {
		const model = modelFrom(epoch);

		if (model !== null) {
			models.set(...model);
		}

		const observation = observationFrom(epoch);

		if (observation !== null) {
			observations.push(observation);
		}
	}

	const currentFit = observations.at(-1)?.fit;

	if (currentFit === undefined) {
		return [];
	}

	const model = models.get(currentFit);
	const current = observations.filter(
		(observation) => observation.fit === currentFit,
	);

	if (model === undefined || current.length < 2) {
		return [];
	}

	return current.slice(0, -1).flatMap((observation, index) => {
		const next = current[index + 1];
		const elapsed = (next.at - observation.at) / 1000;

		if (elapsed <= 0) {
			return [];
		}

		const afterArrival =
			model.baseline +
			(next.intensity - model.baseline) * Math.exp(model.beta * elapsed);

		if (
			!Number.isFinite(afterArrival) ||
			afterArrival < observation.intensity
		) {
			return [];
		}

		return [
			{
				fromAt: observation.at,
				throughAt: next.at,
				beforeArrival: observation.intensity,
				afterArrival,
				throughIntensity: next.intensity,
				baseline: model.baseline,
				beta: model.beta,
			},
		];
	});
};

/*
hawkesIntensityAt evaluates one reconstructed segment at event time so canvas
resolution changes presentation density without changing the process math.
*/
export const hawkesIntensityAt = (
	segment: HawkesCurveSegment,
	at: number,
): number => {
	const boundedAt = Math.min(Math.max(at, segment.fromAt), segment.throughAt);
	const elapsed = (boundedAt - segment.fromAt) / 1000;

	return (
		segment.baseline +
		(segment.afterArrival - segment.baseline) *
			Math.exp(-segment.beta * elapsed)
	);
};
