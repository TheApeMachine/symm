import { createStore } from "@tanstack/react-store";

import {
	gaugeConfidenceReading,
	gaugeSurpriseReading,
	normalizeWireFrame,
} from "#/components/charts/confidence/gauge-frame";

export const SIGNAL_LABELS: Record<string, string> = {
	hawkes: "Hawkes",
	fluid: "Fluid",
	pumpdump: "Pump",
	causal: "Causal",
	depthflow: "Depth",
	leadlag: "Lead-Lag",
	liquidity: "Liquidity",
	sentiment: "Sentiment",
	toxicity: "Toxicity",
	correlation: "Correlation",
	exhaustion: "Exhaustion",
	prediction: "Prediction",
	cvd: "CVD",
	manifold: "Manifold",
};

export const SIGNAL_SOURCES = Object.keys(SIGNAL_LABELS);

export type SignalReading = {
	source: string;
	confidence: number;
	surprise: number;
	surpriseThreshold: number;
	strength: number;
	elapsed: number;
	category: string;
	activeReadings: number;
	readingsCapacity: number;
	observedAt: number | null;
	bestEffort: boolean;
	gapReason: string;
	samples: number;
	minSamples: number;
	calibrating: boolean;
	calibrated: boolean;
	updatedAt: number;
};

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const finiteCount = (value: unknown): number =>
	Math.max(0, Math.floor(finiteNumber(value) ?? 0));

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

const timestampValue = (value: unknown): number | null => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}

	if (typeof value !== "string") {
		return null;
	}

	const timestamp = Date.parse(value);

	return Number.isFinite(timestamp) ? timestamp : null;
};

/*
parseGaugeFrame normalizes dashboard gauge websocket payloads into signal readings.
*/
export const parseGaugeFrame = (
	frame: Record<string, unknown>,
): SignalReading | null => {
	const normalized = normalizeWireFrame(frame);
	const source =
		typeof normalized.source === "string" ? normalized.source.trim() : "";

	if (source === "") {
		return null;
	}

	const confidence = gaugeConfidenceReading(normalized) ?? 0;
	const surprise = gaugeSurpriseReading(normalized) ?? 0;
	const thresholdReading = finiteNumber(normalized.surprise_threshold);
	const surpriseThreshold =
		thresholdReading !== null ? Math.max(0.1, thresholdReading) : 2;
	const strength = finiteNumber(normalized.strength) ?? 0;
	const elapsed = finiteNumber(normalized.elapsed) ?? 0;
	const category = stringValue(normalized.category);
	const activeReadings = finiteCount(normalized.active_readings);
	const readingsCapacity = finiteCount(normalized.readings_capacity);
	const observedAt = timestampValue(normalized.observed_at);
	const bestEffort = normalized.best_effort === true;
	const gapReason = stringValue(normalized.gap_reason);

	return {
		source: source,
		confidence: confidence,
		surprise: surprise,
		surpriseThreshold: surpriseThreshold,
		strength: strength,
		elapsed: elapsed,
		category: category,
		activeReadings: activeReadings,
		readingsCapacity: readingsCapacity,
		observedAt: observedAt,
		bestEffort: bestEffort,
		gapReason: gapReason,
		samples: finiteCount(normalized.samples),
		minSamples: finiteCount(normalized.min_samples),
		calibrating: normalized.calibrating === true,
		calibrated: normalized.calibrated === true,
		updatedAt: Date.now(),
	};
};

export const warmupProgress = (reading: SignalReading): number => {
	if (reading.minSamples <= 0) {
		return 0;
	}

	return Math.min(100, (reading.samples / reading.minSamples) * 100);
};

export const confidenceMeterValue = (reading: SignalReading): number =>
	Math.min(100, Math.max(0, reading.confidence * 100));

export const surpriseMeterValue = (reading: SignalReading): number => {
	const scale = reading.surpriseThreshold * 3;

	if (scale <= 0) {
		return 0;
	}

	return Math.min(100, (reading.surprise / scale) * 100);
};

export const evidenceMeterValue = (reading: SignalReading): number => {
	if (reading.bestEffort || reading.gapReason !== "") {
		return 0;
	}

	if (reading.observedAt === null) {
		return 0;
	}

	if (reading.strength <= 0) {
		return 0;
	}

	if (reading.elapsed <= 0) {
		return 0;
	}

	if (reading.activeReadings > 0) {
		return 100;
	}

	if (reading.category !== "") {
		return 100;
	}

	return 0;
};

export const freshnessMeterValue = (reading: SignalReading): number => {
	if (reading.observedAt === null || reading.elapsed <= 0) {
		return 0;
	}

	const observedAgeSeconds = Math.max(
		0,
		(reading.updatedAt - reading.observedAt) / 1000,
	);

	if (observedAgeSeconds > reading.elapsed) {
		return 0;
	}

	return 100;
};

export const healthMeterValue = (reading: SignalReading): number => {
	if (reading.calibrating) {
		return warmupProgress(reading);
	}

	if (!reading.calibrated) {
		return 0;
	}

	const evidenceScore = evidenceMeterValue(reading);
	const freshnessScore = freshnessMeterValue(reading);

	if (evidenceScore <= 0 || freshnessScore <= 0) {
		return 0;
	}

	const confidenceScore = confidenceMeterValue(reading);
	const surpriseScore = surpriseMeterValue(reading);
	const operational = (evidenceScore + freshnessScore) / 2;
	const energy = (confidenceScore + surpriseScore) / 2;

	return Math.round(0.65 * operational + 0.35 * energy);
};

export type SignalHealthStatus =
	| "waiting"
	| "calibrating"
	| "fault"
	| "stale"
	| "flat"
	| "healthy";

export const signalHealthStatus = (
	reading: SignalReading | null,
): SignalHealthStatus => {
	if (reading === null) {
		return "waiting";
	}

	if (reading.calibrating) {
		return "calibrating";
	}

	if (!reading.calibrated) {
		return "waiting";
	}

	if (reading.bestEffort || reading.gapReason !== "") {
		return "fault";
	}

	if (evidenceMeterValue(reading) <= 0) {
		return "flat";
	}

	if (freshnessMeterValue(reading) <= 0) {
		return "stale";
	}

	return "healthy";
};

export const isSignalDiagnosticReading = (reading: SignalReading): boolean => {
	if (reading.calibrated || reading.calibrating) {
		return true;
	}

	return reading.samples > 0 || reading.minSamples > 0;
};

export const signalStore = createStore(
	{
		readings: {} as Record<string, SignalReading>,
	},
	({ setState }) => ({
		updateReading: (reading: SignalReading) =>
			setState((prev) => ({
				...prev,
				readings: {
					...prev.readings,
					[reading.source]: reading,
				},
			})),
	}),
);
