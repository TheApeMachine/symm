import { createStore } from "@tanstack/react-store";

import {
	gaugeConfidenceReading,
	gaugeSurpriseReading,
	normalizeWireFrame,
} from "#/components/charts/confidence/gauge-frame";

/** Mirrors logic/sources.go SpectrumSources / SourceCount = 13. */
export const SPECTRUM_SOURCES = [
	"causal",
	"correlation",
	"cvd",
	"depthflow",
	"exhaustion",
	"fluid",
	"hawkes",
	"leadlag",
	"liquidity",
	"manifold",
	"pumpdump",
	"sentiment",
	"toxicity",
] as const;

type SignalSourceDef = {
	id: string;
	label: string;
	compactLabel: string;
};

const SOURCE_DEFS: readonly SignalSourceDef[] = [
	{ id: "causal", label: "Causal", compactLabel: "Causal" },
	{ id: "correlation", label: "Correlation", compactLabel: "Corr" },
	{ id: "cvd", label: "CVD", compactLabel: "CVD" },
	{ id: "depthflow", label: "Depth", compactLabel: "Depth" },
	{ id: "exhaustion", label: "Exhaustion", compactLabel: "Exhaust" },
	{ id: "fluid", label: "Fluid", compactLabel: "Fluid" },
	{ id: "hawkes", label: "Hawkes", compactLabel: "Hawkes" },
	{ id: "leadlag", label: "Lead-Lag", compactLabel: "L-Lag" },
	{ id: "liquidity", label: "Liquidity", compactLabel: "Liquidity" },
	{ id: "manifold", label: "Manifold", compactLabel: "Manifold" },
	{ id: "pumpdump", label: "Pump", compactLabel: "Pump" },
	{ id: "sentiment", label: "Sentiment", compactLabel: "Sent" },
	{ id: "toxicity", label: "Toxicity", compactLabel: "Toxic" },
	{ id: "prediction", label: "Prediction", compactLabel: "Pred" },
	{ id: "resonance", label: "Resonance", compactLabel: "Resonance" },
];

const GAUGE_SOURCE_ORDER = [
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
	"depthflow",
	"leadlag",
	"liquidity",
	"sentiment",
	"toxicity",
	"correlation",
	"exhaustion",
	"prediction",
	"cvd",
	"manifold",
] as const;

export const SIGNAL_LABELS: Record<string, string> = Object.fromEntries(
	SOURCE_DEFS.map((entry) => [entry.id, entry.label]),
);

export const SIGNAL_COMPACT_LABELS: Record<string, string> = Object.fromEntries(
	SOURCE_DEFS.map((entry) => [entry.id, entry.compactLabel]),
);

export const SIGNAL_SOURCES = [...GAUGE_SOURCE_ORDER];

export const ALL_SIGNAL_SOURCES = [...GAUGE_SOURCE_ORDER, "resonance"];

/** Wire keys aligned with ui/publish.go gaugeReadingsFromMeasurements. */
const GAUGE_WIRE_FIELDS = {
	source: "source",
	confidence: "confidence",
	surprise: "surprise",
	strength: "strength",
	elapsed: "elapsed",
	category: "category",
	observedAt: "observed_at",
	calibrated: "calibrated",
	readingsCapacity: "readings_capacity",
	surpriseThreshold: "surprise_threshold",
	activeReadings: "active_readings",
	samples: "samples",
	minSamples: "min_samples",
	calibrating: "calibrating",
	bestEffort: "best_effort",
	gapReason: "gap_reason",
} as const;

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
	const raw = normalizeWireFrame(frame);
	const source = stringValue(raw[GAUGE_WIRE_FIELDS.source]);

	if (source === "") {
		return null;
	}

	const confidence = gaugeConfidenceReading(raw) ?? 0;
	const surprise = gaugeSurpriseReading(raw) ?? 0;
	const thresholdReading = finiteNumber(raw[GAUGE_WIRE_FIELDS.surpriseThreshold]);
	const surpriseThreshold =
		thresholdReading !== null ? Math.max(0.1, thresholdReading) : 2;

	return {
		source: source,
		confidence: confidence,
		surprise: surprise,
		surpriseThreshold: surpriseThreshold,
		strength: finiteNumber(raw[GAUGE_WIRE_FIELDS.strength]) ?? 0,
		elapsed: finiteNumber(raw[GAUGE_WIRE_FIELDS.elapsed]) ?? 0,
		category: stringValue(raw[GAUGE_WIRE_FIELDS.category]),
		activeReadings: finiteCount(raw[GAUGE_WIRE_FIELDS.activeReadings]),
		readingsCapacity: finiteCount(raw[GAUGE_WIRE_FIELDS.readingsCapacity]),
		observedAt: timestampValue(raw[GAUGE_WIRE_FIELDS.observedAt]),
		bestEffort: raw[GAUGE_WIRE_FIELDS.bestEffort] === true,
		gapReason: stringValue(raw[GAUGE_WIRE_FIELDS.gapReason]),
		samples: finiteCount(raw[GAUGE_WIRE_FIELDS.samples]),
		minSamples: finiteCount(raw[GAUGE_WIRE_FIELDS.minSamples]),
		calibrating: raw[GAUGE_WIRE_FIELDS.calibrating] === true,
		calibrated: raw[GAUGE_WIRE_FIELDS.calibrated] === true,
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
