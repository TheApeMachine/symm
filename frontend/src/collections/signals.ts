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

	return {
		source: source,
		confidence: confidence,
		surprise: surprise,
		surpriseThreshold: surpriseThreshold,
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

export const healthMeterValue = (reading: SignalReading): number => {
	if (reading.calibrating) {
		return warmupProgress(reading);
	}

	if (!reading.calibrated) {
		return 0;
	}

	const confidenceScore = confidenceMeterValue(reading);
	const surpriseScore = surpriseMeterValue(reading);

	return Math.round((confidenceScore + surpriseScore) / 2);
};

export type SignalHealthStatus = "waiting" | "calibrating" | "flat" | "healthy";

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

	if (healthMeterValue(reading) < 25) {
		return "flat";
	}

	return "healthy";
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
