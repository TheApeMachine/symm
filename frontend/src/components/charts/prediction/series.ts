import { semanticLayerName } from "#/components/terminal/xray-layers";
import type { Value as ResonanceFrame } from "#/types/value";

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
ReturnHeadTrace keeps the strict-prior direction forecast separate from the
unsupervised hierarchy so calibration cannot be mistaken for reconstruction.
*/
export type ReturnHeadTrace = {
	expected: PredictionSample[];
	upper: PredictionSample[];
	lower: PredictionSample[];
	latestExpected: number | null;
	latestUncertainty: number | null;
	taskRelativePrecision: number | null;
	horizon: number | null;
	reach: number | null;
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
layerProfile collects one hierarchy layer's state/prediction components off a
reading. The solver addressed them layer_<i>_state_<j> and
layer_<i>_prediction_<j>, one scalar layer each.
*/
const layerProfile = (
	metrics: Record<string, number>,
	index: number,
	kind: "state" | "prediction",
): number[] => {
	const values: number[] = [];

	for (const [key, value] of Object.entries(metrics)) {
		if (!key.startsWith(`layer_${index}_${kind}_`)) continue;
		const slot = Number.parseInt(
			key.slice(`layer_${index}_${kind}_`.length),
			10,
		);
		if (Number.isNaN(slot)) continue;
		values[slot] = value;
	}

	return values.map((value) => (Number.isFinite(value) ? value : 0));
};

const layerCount = (metrics: Record<string, number>): number => {
	let count = 0;

	for (const key of Object.keys(metrics)) {
		const match = /^layer_(\d+)_state_\d+$/.exec(key);
		if (match) count = Math.max(count, Number(match[1]) + 1);
	}

	return count;
};

/*
hierarchyTrace derives one time-aligned layer series while preserving the latest
full state profile for inspecting what the aggregate history represents. Each
frame contributes one reading: its layer_<i> scalars are the observation.
*/
const hierarchyTrace = (
	frames: ResonanceFrame[],
	index: number,
	count: number,
): HierarchyTrace => {
	const reconstructs = index < count - 1;
	const latest = frames.at(-1);
	const latestMetrics = latest ? latest.metrics() : {};

	return {
		index,
		label: `L${index} · ${semanticLayerName(index, count)}`,
		kind: reconstructs ? "reconstruction" : "context",
		values: frames.map((frame) => {
			const metrics = frame.metrics();
			const state = layerProfile(metrics, index, "state");
			if (state.length === 0) return null;

			return reconstructs
				? reconstructionError(state, layerProfile(metrics, index, "prediction"))
				: vectorNorm(state);
		}),
		state: layerProfile(latestMetrics, index, "state"),
		prediction: reconstructs
			? layerProfile(latestMetrics, index, "prediction")
			: null,
	};
};

/*
returnHeadTrace derives the signed forecast and, where the solver publishes one,
the residual band around it. The reading carries the head's scalar layers:
forward_curve_0 is the signed expectation, task_relative_precision its
readiness. It used to be read from a forecastValidity block alongside an
incremental MSE, a skill lower bound and a calibration count — none of which
appear on the wire, so the lane could only ever report "CALIBRATING" beside four
em-dashes no matter how well the head was doing.
*/
const returnHeadTrace = (frames: ResonanceFrame[]): ReturnHeadTrace => {
	const latest = frames.at(-1);
	const latestMetrics = latest ? latest.metrics() : {};
	const expected = frames.map((frame) =>
		finiteNumber(frame.metrics().forward_curve_0),
	);

	return {
		expected,
		upper: expected.map((value) => value),
		lower: expected.map((value) => value),
		latestExpected: finiteNumber(latestMetrics.forward_curve_0),
		latestUncertainty: null,
		taskRelativePrecision: finiteNumber(latestMetrics.task_relative_precision),
		horizon: null,
		reach: null,
		samples: null,
		ready: finiteNumber(latestMetrics.task_relative_precision) !== null,
	};
};

/*
predictiveCodingSeries exposes all emitted hierarchy layers and the return head
without collapsing five-dimensional states into misleading signed means.
*/
export const predictiveCodingSeries = (
	frames: ResonanceFrame[],
): PredictiveCodingSeries => {
	const latest = frames.at(-1);
	const count = latest ? layerCount(latest.metrics()) : 0;

	return {
		layers: Array.from({ length: count }, (_, index) =>
			hierarchyTrace(frames, index, count),
		),
		returnHead: returnHeadTrace(frames),
	};
};
