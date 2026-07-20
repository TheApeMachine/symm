import type { ResonanceFrame } from "#/collections/types";
import { semanticLayerName } from "#/components/terminal/xray-layers";

/*
PredictionSample preserves an observation epoch in a chart trace. A null value
is an explicit wire-data gap and is rendered as a break rather than as zero.
*/
export type PredictionSample = number | null;

/*
HierarchyTrace describes one settled predictive-coding layer. Reconstruction
layers carry adjacent top-down error; the top layer carries contextual activity.
*/
export type HierarchyTrace = {
	index: number;
	label: string;
	kind: "reconstruction" | "context";
	values: PredictionSample[];
	state: number[];
	prediction: number[] | null;
};

/*
ReturnHeadTrace keeps the strict-prior return forecast separate from the
unsupervised hierarchy so calibration cannot be mistaken for reconstruction.
*/
export type ReturnHeadTrace = {
	expected: PredictionSample[];
	upper: PredictionSample[];
	lower: PredictionSample[];
	latestExpected: number | null;
	latestUncertainty: number | null;
	mse: number | null;
	skillLowerBound: number | null;
	samples: number | null;
	ready: boolean;
};

/*
PredictiveCodingSeries is the complete chart-facing view of the resonance
subsystem: every hierarchy layer plus the independent supervised return head.
*/
export type PredictiveCodingSeries = {
	layers: HierarchyTrace[];
	returnHead: ReturnHeadTrace;
};

/*
finiteNumber rejects absent and non-finite wire metrics without manufacturing a
numeric substitute that would create a false observation in the chart.
*/
const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

/*
finiteVector accepts a complete numeric state vector. Partial vectors are not a
valid predictive-coding layer and remain absent from the derived series.
*/
const finiteVector = (value: unknown): number[] =>
	Array.isArray(value) &&
	value.length > 0 &&
	value.every((entry) => typeof entry === "number" && Number.isFinite(entry))
		? value
		: [];

/*
vectorNorm measures layer activity without averaging signed components that can
cancel one another.
*/
const vectorNorm = (values: number[]): number | null => {
	if (values.length === 0) {
		return null;
	}

	return Math.hypot(...values);
};

/*
reconstructionError calculates the same Euclidean residual norm exported by the
backend predictive-coding manifold for an adjacent generative link.
*/
export const reconstructionError = (
	stateValue: unknown,
	predictionValue: unknown,
): number | null => {
	const state = finiteVector(stateValue);
	const prediction = finiteVector(predictionValue);

	if (state.length === 0 || state.length !== prediction.length) {
		return null;
	}

	return Math.hypot(
		...state.map((value, index) => value - (prediction[index] as number)),
	);
};

/*
hierarchyTrace derives one time-aligned layer series while preserving the latest
full state profile for inspecting what the aggregate history represents.
*/
const hierarchyTrace = (
	frames: ResonanceFrame[],
	index: number,
	count: number,
): HierarchyTrace => {
	const reconstructs = index < count - 1;
	const latest = frames.at(-1)?.layers?.[index];

	return {
		index,
		label: `L${index} · ${semanticLayerName(index, count)}`,
		kind: reconstructs ? "reconstruction" : "context",
		values: frames.map((frame) => {
			const layer = frame.layers?.[index];

			return reconstructs
				? reconstructionError(layer?.state, layer?.prediction)
				: vectorNorm(finiteVector(layer?.state));
		}),
		state: finiteVector(latest?.state),
		prediction: reconstructs ? finiteVector(latest?.prediction) : null,
	};
};

/*
returnHeadTrace derives the signed forecast and empirical residual-uncertainty
band, along with the calibration evidence that determines readiness.
*/
const returnHeadTrace = (frames: ResonanceFrame[]): ReturnHeadTrace => {
	const latest = frames.at(-1);
	const expected = frames.map((frame) => finiteNumber(frame.expectedReturn));
	const uncertainty = frames.map((frame) => finiteNumber(frame.uncertainty));

	return {
		expected,
		upper: expected.map((value, index) => {
			const spread = uncertainty[index];
			return value === null || spread === null || spread < 0
				? null
				: value + spread;
		}),
		lower: expected.map((value, index) => {
			const spread = uncertainty[index];
			return value === null || spread === null || spread < 0
				? null
				: value - spread;
		}),
		latestExpected: finiteNumber(latest?.expectedReturn),
		latestUncertainty: finiteNumber(latest?.uncertainty),
		mse: finiteNumber(latest?.incrementalMSE),
		skillLowerBound: finiteNumber(latest?.incrementalSkillLowerBound),
		samples: finiteNumber(latest?.calibrationSamples),
		ready: latest?.returnReady === true,
	};
};

/*
predictiveCodingSeries exposes all emitted hierarchy layers and the return head
without collapsing five-dimensional states into misleading signed means.
*/
export const predictiveCodingSeries = (
	frames: ResonanceFrame[],
): PredictiveCodingSeries => {
	const layerCount = frames.at(-1)?.layers?.length ?? 0;

	return {
		layers: Array.from({ length: layerCount }, (_, index) =>
			hierarchyTrace(frames, index, layerCount),
		),
		returnHead: returnHeadTrace(frames),
	};
};
