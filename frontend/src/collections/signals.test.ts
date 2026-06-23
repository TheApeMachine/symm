import { beforeEach, describe, expect, it } from "vitest";

import {
	ALL_SIGNAL_SOURCES,
	confidenceMeterValue,
	evidenceMeterValue,
	freshnessMeterValue,
	healthMeterValue,
	isSignalDiagnosticReading,
	parseGaugeFrame,
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	type SignalReading,
	SPECTRUM_SOURCES,
	signalHealthStatus,
	signalStore,
	surpriseMeterValue,
	warmupProgress,
} from "#/collections/signals";

describe("parseGaugeFrame", () => {
	it("returns null when source is missing", () => {
		expect(parseGaugeFrame({ confidence: 0.5 })).toBeNull();
	});

	it("normalizes gauge websocket payloads", () => {
		const reading = parseGaugeFrame({
			source: "fluid",
			confidence: 0.42,
			surprise: 1.8,
			surprise_threshold: 2,
			samples: 120,
			min_samples: 240,
			calibrating: true,
			calibrated: false,
		});

		expect(reading).not.toBeNull();
		expect(reading?.source).toBe("fluid");
		expect(reading?.confidence).toBe(0.42);
		expect(reading?.surprise).toBe(1.8);
		expect(reading?.surpriseThreshold).toBe(2);
		expect(reading?.samples).toBe(120);
		expect(reading?.minSamples).toBe(240);
		expect(reading?.calibrating).toBe(true);
		expect(reading?.calibrated).toBe(false);
	});

	it("normalizes bulk story measurements with PascalCase fields", () => {
		const reading = parseGaugeFrame({
			Source: "leadlag",
			Confidence: 0.55,
			Surprise: 1.2,
		});

		expect(reading).not.toBeNull();
		expect(reading?.source).toBe("leadlag");
		expect(reading?.confidence).toBe(0.55);
		expect(reading?.surprise).toBe(1.2);
		expect(reading?.calibrated).toBe(false);
	});

	it("reads surprise from nested cognition attributes", () => {
		const reading = parseGaugeFrame({
			source: "fluid",
			confidence: 0.55,
			cognition: {
				surprise: { value: 2.4, threshold: 1.8 },
			},
		});

		expect(reading).not.toBeNull();
		expect(reading?.surprise).toBe(2.4);
		expect(reading?.surpriseThreshold).toBe(1.8);
	});

	it("preserves bulk measurement evidence without declaring calibration", () => {
		const observedAt = new Date(Date.now() - 500).toISOString();
		const reading = parseGaugeFrame({
			Source: "leadlag",
			Confidence: 0.55,
			Surprise: 1.2,
			Strength: 0.48,
			Elapsed: 5,
			Category: "synchronized_drift",
			ObservedAt: observedAt,
		});

		expect(reading).not.toBeNull();
		expect(reading?.calibrated).toBe(false);
		expect(reading?.strength).toBe(0.48);
		expect(reading?.category).toBe("synchronized_drift");
		expect(reading?.observedAt).toBe(Date.parse(observedAt));
		expect(reading === null ? null : isSignalDiagnosticReading(reading)).toBe(
			false,
		);
		expect(reading === null ? null : signalHealthStatus(reading)).toBe(
			"healthy",
		);
	});

	it("accepts calibrated aggregate gauge evidence as healthy", () => {
		const reading = parseGaugeFrame({
			source: "leadlag",
			confidence: 0.55,
			surprise: 3,
			strength: 0.48,
			elapsed: 5,
			active_readings: 4,
			readings_capacity: 8,
			observed_at: new Date(Date.now() - 500).toISOString(),
			calibrated: true,
		});

		expect(reading).not.toBeNull();
		expect(reading?.calibrated).toBe(true);
		expect(reading === null ? null : isSignalDiagnosticReading(reading)).toBe(
			true,
		);
		expect(reading === null ? null : signalHealthStatus(reading)).toBe(
			"healthy",
		);
	});

	it("maps measurement artifact fields into gauge readings", () => {
		const observedAt = Date.now() - 250;
		const reading = parseGaugeFrame({
			origin: "fluid",
			scope: "BTC/USD",
			timestamp: observedAt,
			output: {
				confidence: 0.71,
				strength: 0.36,
				category: 2,
				surprise: 2.4,
				elapsed: 30,
			},
		});

		expect(reading).not.toBeNull();
		expect(reading?.source).toBe("fluid");
		expect(reading?.confidence).toBe(0.71);
		expect(reading?.surprise).toBe(2.4);
		expect(reading?.strength).toBe(0.36);
		expect(reading?.elapsed).toBe(30);
		expect(reading?.category).toBe("2");
		expect(reading?.observedAt).toBe(observedAt);
	});
});

describe("signal source registry", () => {
	it("lists the 13 spectrum sources in backend order", () => {
		expect(SPECTRUM_SOURCES).toEqual([
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
		]);
	});

	it("exposes gauge telemetry sources plus resonance for heatmaps", () => {
		expect(SIGNAL_SOURCES).toHaveLength(14);
		expect(SIGNAL_SOURCES).not.toContain("resonance");
		expect(ALL_SIGNAL_SOURCES).toHaveLength(15);
		expect(ALL_SIGNAL_SOURCES).toContain("resonance");
	});

	it("provides canonical labels for every registered source", () => {
		for (const source of ALL_SIGNAL_SOURCES) {
			expect(SIGNAL_LABELS[source]).toBeTruthy();
		}

		expect(SIGNAL_LABELS.exhaustion).toBe("Exhaustion");
		expect(SIGNAL_LABELS.resonance).toBe("Resonance");
	});
});

const sampleReading = (): SignalReading => ({
	source: "fluid",
	confidence: 0.42,
	surprise: 1.8,
	surpriseThreshold: 2,
	strength: 0.4,
	elapsed: 60,
	category: "laminar",
	activeReadings: 1,
	readingsCapacity: 8,
	observedAt: Date.now(),
	bestEffort: false,
	gapReason: "",
	samples: 0,
	minSamples: 0,
	calibrating: false,
	calibrated: true,
	updatedAt: Date.now(),
});

describe("signalStore", () => {
	beforeEach(() => {
		signalStore.setState({ readings: {} });
	});

	it("stores readings by source", () => {
		const reading = sampleReading();

		signalStore.actions.updateReading(reading);

		expect(signalStore.state.readings.fluid).toEqual(reading);
	});

	it("replaces prior readings for the same source", () => {
		signalStore.actions.updateReading(sampleReading());
		signalStore.actions.updateReading({
			...sampleReading(),
			confidence: 0.9,
		});

		expect(signalStore.state.readings.fluid?.confidence).toBe(0.9);
	});
});

describe("signal diagnostics meters", () => {
	const calibratedReading = {
		source: "hawkes",
		confidence: 0.6,
		surprise: 3,
		surpriseThreshold: 2,
		strength: 0.4,
		elapsed: 60,
		category: "laminar",
		activeReadings: 1,
		readingsCapacity: 8,
		observedAt: Date.now(),
		bestEffort: false,
		gapReason: "",
		samples: 0,
		minSamples: 0,
		calibrating: false,
		calibrated: true,
		updatedAt: Date.now(),
	};

	it("computes confidence and surprise meters", () => {
		expect(confidenceMeterValue(calibratedReading)).toBe(60);
		expect(surpriseMeterValue(calibratedReading)).toBe(50);
		expect(evidenceMeterValue(calibratedReading)).toBe(100);
		expect(freshnessMeterValue(calibratedReading)).toBe(100);
	});

	it("uses warmup progress while calibrating", () => {
		const reading = {
			...calibratedReading,
			calibrating: true,
			calibrated: false,
			samples: 50,
			minSamples: 200,
		};

		expect(warmupProgress(reading)).toBe(25);
		expect(healthMeterValue(reading)).toBe(25);
		expect(signalHealthStatus(reading)).toBe("calibrating");
	});

	it("marks low-energy calibrated signals as healthy when evidence is present", () => {
		const reading = {
			...calibratedReading,
			confidence: 0.05,
			surprise: 0.1,
		};

		expect(signalHealthStatus(reading)).toBe("healthy");
		expect(healthMeterValue(reading)).toBeGreaterThan(25);
	});

	it("keeps threshold-edge signals healthy when evidence is present", () => {
		const reading = {
			...calibratedReading,
			confidence: 0.25,
			surprise: 6,
		};

		expect(healthMeterValue(reading)).toBeGreaterThan(25);
		expect(signalHealthStatus(reading)).toBe("healthy");
	});

	it("marks measurements older than their observation window as stale", () => {
		const reading = {
			...calibratedReading,
			elapsed: 1,
			observedAt: Date.now() - 2000,
		};

		expect(freshnessMeterValue(reading)).toBe(0);
		expect(healthMeterValue(reading)).toBe(0);
		expect(signalHealthStatus(reading)).toBe("stale");
	});

	it("does not let high confidence mask missing evidence", () => {
		const reading = {
			...calibratedReading,
			confidence: 1,
			surprise: 6,
			strength: 0,
			elapsed: 60,
			category: "",
			activeReadings: 0,
			observedAt: null,
		};

		expect(evidenceMeterValue(reading)).toBe(0);
		expect(healthMeterValue(reading)).toBe(0);
		expect(signalHealthStatus(reading)).toBe("flat");
	});

	it("reports explicit measurement gaps as faults", () => {
		const reading = {
			...calibratedReading,
			bestEffort: true,
			gapReason: "insufficient_depth",
		};

		expect(evidenceMeterValue(reading)).toBe(0);
		expect(signalHealthStatus(reading)).toBe("fault");
	});

	it("marks raw measurement activity as flat instead of waiting", () => {
		const reading = {
			...calibratedReading,
			calibrated: false,
			elapsed: 0,
			strength: 0,
			activeReadings: 0,
			category: "",
		};

		expect(healthMeterValue(reading)).toBe(0);
		expect(signalHealthStatus(reading)).toBe("flat");
	});
});
