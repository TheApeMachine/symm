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

const sumValues = (...values: Array<number | null>): number | null => {
	const available = values.filter((value): value is number => value !== null);

	if (available.length === 0) {
		return null;
	}

	return available.reduce((sum, value) => sum + value, 0);
};

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

		if (
			measurement.metric === metric &&
			(measurement.side ?? "") === side &&
			Number.isFinite(measurement.raw)
		) {
			return measurement.raw ?? null;
		}

		const raw = measurement.metrics?.[key];

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
*/
export const xrayLayersFromResonance = (
	frame: ResonanceFrame | null | undefined,
): XrayLayer[] => {
	const layers = frame?.layers ?? [];

	return layers.map((layer, index) => {
		const state = numberArray(layer.state);
		const prediction = numberArray(layer.prediction);

		return {
			index,
			label: `L${index} · ${semanticLayerName(index, layers.length)}`,
			state,
			error_norm: layerError(state, prediction, frame?.surprise),
		};
	});
};

const hawkesMetrics = (epoch: Measurement[]): HawkesMetrics => {
	const buyIntensity =
		measurementRaw(epoch, "conditional_intensity", "buy") ??
		measurementRaw(epoch, "arrival_rate", "buy");
	const sellIntensity =
		measurementRaw(epoch, "conditional_intensity", "sell") ??
		measurementRaw(epoch, "arrival_rate", "sell");
	const radius = measurementRaw(epoch, "spectral_radius");

	return {
		intensity: sumValues(buyIntensity, sellIntensity),
		branching: radius,
		radius,
		asymmetry: null,
		buyIntensity,
		sellIntensity,
		exo: sumValues(
			measurementRaw(epoch, "baseline_intensity", "buy"),
			measurementRaw(epoch, "baseline_intensity", "sell"),
		),
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

	return epoch === undefined
		? {
				intensity: null,
				branching: null,
				radius: null,
				asymmetry: null,
				buyIntensity: null,
				sellIntensity: null,
				exo: null,
			}
		: hawkesMetrics(epoch);
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
latentPointsFromFrames projects each symbol's latest resonance latent pair for
the universe scatter without inventing coordinates.
*/
export const latentPointsFromFrames = (
	frames: ResonanceFrame[],
): LatentPoint[] => {
	const latest = new Map<string, ResonanceFrame>();

	for (const frame of frames) {
		latest.set(frame.symbol, frame);
	}

	return [...latest.entries()].flatMap(([symbol, frame]) => {
		const latent = numberArray(frame.latent);

		if (latent.length < 2) {
			return [];
		}

		return [
			{
				key: `${symbol}:${String(frame.at ?? "")}`,
				symbol,
				x: latent[0] ?? 0,
				y: latent[1] ?? 0,
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
