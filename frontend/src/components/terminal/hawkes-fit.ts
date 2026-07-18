import type { CircularBuffer } from "#/collections/circular";
import type { Measurement, MeasurementEpoch } from "#/collections/measurements";

/*
HawkesModel is the fitted univariate process shared by one parameter epoch.
*/
export type HawkesModel = {
	baseline: number;
	beta: number;
};

/*
HawkesObservation is the intensity emitted for one market event arrival.
*/
export type HawkesObservation = {
	at: number;
	intensity: number;
	fit: string;
	symbol: string;
};

/*
HawkesFitState is the active model plus arrival intensities for one fit origin.
*/
export type HawkesFitState = {
	model: HawkesModel;
	observations: HawkesObservation[];
};

type RetainedHawkesFit = {
	model: HawkesModel;
	raw: Map<string, number>;
};

const retainedFits = new Map<string, RetainedHawkesFit>();

/*
resetHawkesFitRetention clears retained fits so tests start from an empty cache.
*/
export const resetHawkesFitRetention = (): void => {
	retainedFits.clear();
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
fitKey identifies the retained Hawkes fit by its origin. Conditional intensities
widen scale.through to the evaluation horizon while model parameters keep the
fit end, so only scale.from is stable across both evidence kinds.
*/
const fitKey = (measurement: Measurement | undefined): string =>
	measurement?.scale?.from ?? "";

/*
parameterKey addresses one directional model metric inside a retained fit.
*/
const parameterKey = (metric: string, side: string): string =>
	`${metric}\u0000${side}`;

/*
retentionKey scopes one retained fit to its symbol so concurrent pairs cannot
share model state.
*/
const retentionKey = (symbol: string, fit: string): string =>
	`${symbol}\u0000${fit}`;

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
retainHawkesModelEpoch caches one complete model epoch so later intensity-only
ticks can still reconstruct the decay curve and branching metrics for that fit
origin, including when the sparse ModelUpdated slot has left the buffer.
*/
export const retainHawkesModelEpoch = (epoch: MeasurementEpoch): void => {
	const model = modelFrom(epoch);

	if (model === null) {
		return;
	}

	const [fit, hawkesModel] = model;
	const symbol = epoch.readings[0]?.symbol ?? "";

	if (symbol === "") {
		return;
	}

	const values = new Map<string, number>();

	for (const measurement of epoch.readings) {
		if (fitKey(measurement) !== fit || !Number.isFinite(measurement.raw)) {
			continue;
		}

		values.set(
			parameterKey(measurement.metric, measurement.side ?? ""),
			measurement.raw,
		);
	}

	const key = retentionKey(symbol, fit);
	const prefix = `${symbol}\u0000`;

	for (const retained of retainedFits.keys()) {
		if (retained.startsWith(prefix) && retained !== key) {
			retainedFits.delete(retained);
		}
	}

	retainedFits.set(key, {
		model: hawkesModel,
		raw: values,
	});
};

/*
observationFrom reads the total intensity for one market event arrival.
*/
const observationFrom = (epoch: MeasurementEpoch): HawkesObservation | null => {
	const buy = reading(epoch, "conditional_intensity", "buy");
	const sell = reading(epoch, "conditional_intensity", "sell");
	const at = Date.parse(epoch.at);
	const buyFit = fitKey(buy);
	const sellFit = fitKey(sell);
	const symbol = buy?.symbol ?? sell?.symbol ?? "";

	if (
		buy === undefined ||
		sell === undefined ||
		!Number.isFinite(buy.raw) ||
		!Number.isFinite(sell.raw) ||
		!Number.isFinite(at) ||
		buyFit === "" ||
		buyFit !== sellFit ||
		symbol === ""
	) {
		return null;
	}

	return { at, intensity: buy.raw + sell.raw, fit: buyFit, symbol };
};

/*
latestHawkesRaw carries fitted parameters forward within the active fit epoch.
Matching fit origin prevents a missing current parameter from silently falling
back to a stale model, while retention survives sparse ModelUpdated epochs.
*/
export const latestHawkesRaw = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
	metric: string,
	side = "",
): number | null => {
	const epochs = buffer?.values() ?? [];
	let currentFit = "";
	let symbol = "";

	for (const epoch of epochs) {
		retainHawkesModelEpoch(epoch);
	}

	for (let index = epochs.length - 1; index >= 0; index -= 1) {
		const observation = observationFrom(epochs[index]);

		if (observation !== null) {
			currentFit = observation.fit;
			symbol = observation.symbol;
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

	return (
		retainedFits
			.get(retentionKey(symbol, currentFit))
			?.raw.get(parameterKey(metric, side)) ?? null
	);
};

/*
collectFitState gathers the active fit model and its intensity observations.
*/
export const collectFitState = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): HawkesFitState | null => {
	const epochs = buffer?.values() ?? [];
	const models = new Map<string, HawkesModel>();
	const observations: HawkesObservation[] = [];

	for (const epoch of epochs) {
		retainHawkesModelEpoch(epoch);
		const model = modelFrom(epoch);

		if (model !== null) {
			models.set(...model);
		}

		const observation = observationFrom(epoch);

		if (observation !== null) {
			observations.push(observation);
		}
	}

	const latest = observations.at(-1);

	if (latest === undefined) {
		return null;
	}

	const model =
		models.get(latest.fit) ??
		retainedFits.get(retentionKey(latest.symbol, latest.fit))?.model;

	if (model === undefined) {
		return null;
	}

	const current = observations.filter(
		(observation) => observation.fit === latest.fit,
	);

	if (current.length === 0) {
		return null;
	}

	return { model, observations: current };
};
