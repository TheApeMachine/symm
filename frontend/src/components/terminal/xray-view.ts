import type { Measurement, ResonanceFrame } from "#/collections/types";
import { semanticLayerName } from "#/components/terminal/xray-layers";
import { requirePositiveLength } from "#/lib/domain";

export type HawkesMetrics = {
	intensity: number | null;
	branching: number | null;
	radius: number | null;
	asymmetry: number | null;
	buyIntensity: number | null;
	sellIntensity: number | null;
	exo: number | null;
};

export type LatentPoint = {
	key: string;
	symbol: string;
	x: number;
	y: number;
	category: string;
};

export type XrayLayer = {
	index: number;
	label: string;
	state: number[];
	prediction: number[];
	error_norm: number;
};

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter((item): item is number => typeof item === "number")
		: [];

const measurementEpochs = (measurements: Measurement[]): Measurement[][] => {
	const epochs = new Map<string, Measurement[]>();

	for (const measurement of measurements) {
		const epoch = epochs.get(measurement.at) ?? [];
		epoch.push(measurement);
		epochs.set(measurement.at, epoch);
	}

	return [...epochs.values()];
};

const measurementRaw = (
	epoch: Measurement[],
	metric: string,
	side = "",
): number | null => {
	const key = side === "" ? metric : `${metric}:${side}`;

	for (let index = epoch.length - 1; index >= 0; index -= 1) {
		const measurement = epoch[index];
		const raw = measurement.metrics?.[key]?.raw;

		if (typeof raw === "number" && Number.isFinite(raw)) {
			return raw;
		}
	}

	return null;
};

const layerError = (
	state: number[],
	prediction: number[],
	surprise: number | undefined,
): number => {
	if (state.length > 0 && prediction.length === state.length) {
		requirePositiveLength(state.length, "xray mean absolute error");

		const total = state.reduce(
			(sum, value, index) => sum + Math.abs(value - (prediction[index] ?? 0)),
			0,
		);

		return total / state.length;
	}

	const fallback = finite(surprise);

	return fallback === null ? 0 : Math.min(1, Math.abs(fallback));
};

/*
xrayLayersFromResonance maps backend resonance layers into hierarchy rows so the
predictive-coding panel never invents structure from manifold density.

The network reports its own prediction error per layer. That reading wins over
the mean absolute difference recomputed here, which only stands in for layers
published before errorNorm existed.
*/
export const xrayLayersFromResonance = (
	frame: ResonanceFrame | Record<string, unknown> | null | undefined,
): XrayLayer[] => {
	const layers =
		(frame?.layers as Array<Record<string, unknown>> | undefined) ?? [];

	return layers.map((layer, index) => {
		const state = numberArray(layer.state);
		const prediction = numberArray(layer.prediction);
		const reported = finite(layer.errorNorm ?? layer.error_norm);

		return {
			index,
			label: `L${index} · ${semanticLayerName(index, layers.length)}`,
			state,
			prediction,
			error_norm:
				reported ??
				layerError(state, prediction, frame?.surprise as number | undefined),
		};
	});
};

const hawkesMetrics = (epoch: Measurement[]): HawkesMetrics => {
	const aggregateIntensity =
		measurementRaw(epoch, "conditional_intensity") ??
		measurementRaw(epoch, "arrival_rate");
	const buyIntensity =
		measurementRaw(epoch, "conditional_intensity", "buy") ??
		measurementRaw(epoch, "arrival_rate", "buy");
	const sellIntensity =
		measurementRaw(epoch, "conditional_intensity", "sell") ??
		measurementRaw(epoch, "arrival_rate", "sell");
	const radius =
		measurementRaw(epoch, "branching_spectral_radius") ??
		measurementRaw(epoch, "spectral_radius");

	const aggregateBg = measurementRaw(epoch, "background_rate");
	const buyBg = measurementRaw(epoch, "background_rate", "buy");
	const sellBg = measurementRaw(epoch, "background_rate", "sell");

	const intensity =
		aggregateIntensity ??
		(buyIntensity !== null && sellIntensity !== null
			? buyIntensity + sellIntensity
			: (buyIntensity ?? sellIntensity));

	const exo =
		aggregateBg ??
		(buyBg !== null && sellBg !== null ? buyBg + sellBg : (buyBg ?? sellBg));

	return {
		intensity,
		branching: radius,
		radius,
		asymmetry: null,
		buyIntensity,
		sellIntensity,
		exo,
	};
};

/*
hawkesMetricsFromFrames reads Hawkes intensity metrics from a flat measurement
history for one symbol/source without store access.
*/
export const hawkesMetricsFromFrames = (
	frames: Measurement[],
): HawkesMetrics => {
	const epoch = measurementEpochs(frames).at(-1);

	if (epoch === undefined) {
		return {
			intensity: null,
			branching: null,
			radius: null,
			asymmetry: null,
			buyIntensity: null,
			sellIntensity: null,
			exo: null,
		};
	}

	return hawkesMetrics(epoch);
};

/*
hawkesMetricsFromBuffer is the array-facing alias used by paint paths that
already filtered Hawkes rows from a worker snapshot.
*/
export const hawkesMetricsFromBuffer = (
	frames: Measurement[] | undefined,
): HawkesMetrics => hawkesMetricsFromFrames(frames ?? []);

export const hawkesEventCount = (frames: Measurement[] | undefined): number =>
	frames?.length ?? 0;

/*
intensitySeriesFromRingRows collapses per-epoch intensity samples into a
plotted series, sorted by arrival. A ring buffer holds one row per emission,
not one row per epoch, and most emissions for an epoch carry an unrelated
Hawkes binding (compensator, excitation decay, offspring, ...) rather than
intensity — a caller that pushed a placeholder for every row lacking the
metric turned a sparse impulse train into linearly-interpolated triangles.
Only epochs that actually reported the metric are included, keeping the last
sample per epoch when more than one row for the same epoch reports it.
*/
export const intensitySeriesFromRingRows = (
	rows: Array<{ at: bigint; raw: number }>,
): number[] => {
	const byEpoch = new Map<bigint, number>();

	for (const row of rows) {
		byEpoch.set(row.at, row.raw);
	}

	return [...byEpoch.entries()]
		.sort((left, right) =>
			left[0] < right[0] ? -1 : left[0] > right[0] ? 1 : 0,
		)
		.map(([, value]) => value);
};

/*
latentPointsFromFrames projects each symbol's latest resonance latent pair for
the universe scatter without inventing coordinates.
*/
export const latentPointsFromFrames = (
	frames: Array<ResonanceFrame | Record<string, unknown>>,
): LatentPoint[] => {
	const latest = new Map<string, ResonanceFrame | Record<string, unknown>>();

	for (const frame of frames) {
		const symbol = stringValue(frame.symbol);

		if (symbol !== "") {
			latest.set(symbol, frame);
		}
	}

	return [...latest.entries()].flatMap(([symbol, frame]) => {
		/*
			Every carrier publishes the two leading components of its settled state
			as an embedding; only the focused one carries the full latent vector.
			Reading the embedding first is what makes this a cross-section rather
			than a single point, and the fallback keeps a frame that predates the
			field plotting.
		*/
		const embedding = numberArray(frame.embedding);
		const position =
			embedding.length >= 2 ? embedding : numberArray(frame.latent);

		if (position.length < 2) {
			return [];
		}

		return [
			{
				key: `${symbol}:${String(frame.at ?? "")}`,
				symbol,
				x: position[0] ?? 0,
				y: position[1] ?? 0,
				category: stringValue(frame.category),
			},
		];
	});
};

export const cascadeLabel = (
	branching: number | null,
): { label: string; color: string } => {
	if (branching === null) {
		return { label: "—", color: "var(--f4)" };
	}

	if (branching > 0.85) {
		return { label: "critical", color: "var(--down)" };
	}

	if (branching > 0.6) {
		return { label: "elevated", color: "var(--warn)" };
	}

	return { label: "stable", color: "var(--up)" };
};

export const formatMetric = (value: number | null, digits = 3): string =>
	value === null ? "—" : value.toFixed(digits);

export const signedMetric = (value: number | null, digits = 3): string => {
	if (value === null) {
		return "—";
	}

	return `${value >= 0 ? "+" : "−"}${Math.abs(value).toFixed(digits)}`;
};

export const manifoldReading = (
	frame: Record<string, unknown> | null | undefined,
): Record<string, unknown> | null => {
	if (frame === null || frame === undefined) {
		return null;
	}

	const reading = frame.reading;

	if (
		reading !== null &&
		typeof reading === "object" &&
		!Array.isArray(reading)
	) {
		return reading as Record<string, unknown>;
	}

	return frame;
};

export const finiteMetric = finite;
export const stringMetric = stringValue;

/*
Retained universe maps across sparse telemetry updates.
*/
const retainedResonance = new Map<string, Record<string, unknown>>();
const retainedHawkes = new Map<string, Record<string, number>>();

export const retainResonanceRow = (
	symbol: string,
	row: Record<string, unknown>,
) => {
	if (!symbol) return;
	const existing = retainedResonance.get(symbol) ?? {};
	retainedResonance.set(symbol, { ...existing, ...row, symbol });
};

export const getRetainedResonance = (
	symbol?: string,
): Record<string, unknown> | null => {
	if (!symbol) return null;
	return retainedResonance.get(symbol) ?? null;
};

export const getAllRetainedResonance = (): Array<Record<string, unknown>> => [
	...retainedResonance.values(),
];

export const retainHawkesMetric = (
	symbol: string,
	metric: string,
	raw: number,
) => {
	if (!symbol || !metric || !Number.isFinite(raw)) return;
	const current = retainedHawkes.get(symbol) ?? {};
	current[metric] = raw;
	retainedHawkes.set(symbol, current);
};

export const getRetainedHawkes = (symbol?: string): Record<string, number> => {
	if (!symbol) return {};
	return retainedHawkes.get(symbol) ?? {};
};

export const clearRetainedTelemetry = () => {
	retainedResonance.clear();
	retainedHawkes.clear();
};
