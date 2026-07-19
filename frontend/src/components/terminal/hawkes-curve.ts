import type { CircularBuffer } from "#/collections/circular";
import type { MeasurementEpoch } from "#/collections/types";
import {
	collectFitState,
	type HawkesObservation,
} from "#/components/terminal/hawkes-fit";

export {
	latestHawkesRaw,
	resetHawkesFitRetention,
	retainHawkesModelEpoch,
} from "#/components/terminal/hawkes-fit";

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
HawkesSeries is a dense λ(t) path for canvas drawing: impulses at arrivals and
exponential decay between them, including a trailing decay to the present.
*/
export type HawkesSeries = {
	baseline: number;
	beta: number;
	fit: string;
	peakExcess: number;
	fromAt: number;
	throughAt: number;
	samples: number[];
	events: number[];
};

/*
intensityAt evaluates μ + (λᵢ − μ)·e^(−β(t−tᵢ)) after the latest arrival at or
before t, matching the mockup's continuous decay sampler.
*/
const intensityAt = (
	observations: HawkesObservation[],
	baseline: number,
	beta: number,
	at: number,
): number => {
	let latest: HawkesObservation | undefined;

	for (const observation of observations) {
		if (observation.at > at) {
			break;
		}

		latest = observation;
	}

	if (latest === undefined) {
		return baseline;
	}

	const elapsed = Math.max(0, (at - latest.at) / 1000);

	return baseline + (latest.intensity - baseline) * Math.exp(-beta * elapsed);
};

/*
hawkesSeriesFromBuffer builds a dense intensity series over a window long enough
for several half-lives of decay so impulses do not collapse to a single pixel.
*/
export const hawkesSeriesFromBuffer = (
	buffer: CircularBuffer<MeasurementEpoch> | MeasurementEpoch[] | undefined,
	now = Date.now(),
	sampleCount = 220,
): HawkesSeries | null => {
	const state = collectFitState(buffer);

	if (state === null || sampleCount < 2) {
		return null;
	}

	const { model, observations } = state;
	const lastAt = observations[observations.length - 1]?.at ?? now;
	const throughAt = Math.max(now, lastAt);
	const halfLifeMs = (Math.LN2 / model.beta) * 1000;
	const minWindowMs = Math.max(4_000, halfLifeMs * 6);
	const maxWindowMs = 60_000;
	const firstAt = observations[0]?.at ?? throughAt;
	const windowMs = Math.min(
		maxWindowMs,
		Math.max(minWindowMs, throughAt - firstAt),
	);
	const fromAt = throughAt - windowMs;
	const duration = Math.max(1, throughAt - fromAt);
	const samples = new Array<number>(sampleCount);

	for (let index = 0; index < sampleCount; index += 1) {
		const at = fromAt + (index / (sampleCount - 1)) * duration;
		samples[index] = intensityAt(observations, model.baseline, model.beta, at);
	}

	let peakExcess = 0;

	for (const observation of observations) {
		peakExcess = Math.max(peakExcess, observation.intensity - model.baseline);
	}

	return {
		baseline: model.baseline,
		beta: model.beta,
		fit: observations.at(-1)?.fit ?? "",
		peakExcess: Math.max(0, peakExcess),
		fromAt,
		throughAt,
		samples,
		events: observations.map((observation) => observation.at),
	};
};

/*
hawkesCurveFromBuffer reconstructs identifiable impulse segments between
consecutive arrivals for tests of the exponential kernel identity.
*/
export const hawkesCurveFromBuffer = (
	buffer: CircularBuffer<MeasurementEpoch> | undefined,
): HawkesCurveSegment[] => {
	const state = collectFitState(buffer);

	if (state === null || state.observations.length < 2) {
		return [];
	}

	const { model, observations } = state;

	return observations.slice(0, -1).flatMap((observation, index) => {
		const next = observations[index + 1];
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
