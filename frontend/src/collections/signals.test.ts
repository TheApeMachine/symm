import { describe, expect, it } from "vitest";

import {
	confidenceMeterValue,
	evidenceMeterValue,
	freshnessMeterValue,
	healthMeterValue,
	isSignalDiagnosticReading,
	parseGaugeFrame,
	signalHealthStatus,
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

	it("marks low-energy calibrated signals as flat", () => {
		const reading = {
			...calibratedReading,
			confidence: 0.05,
			surprise: 0.1,
		};

		expect(signalHealthStatus(reading)).toBe("flat");
		expect(healthMeterValue(reading)).toBeLessThan(25);
	});

	it("keeps threshold-edge signals flat", () => {
		const reading = {
			...calibratedReading,
			confidence: 0.25,
			surprise: 6,
		};

		expect(healthMeterValue(reading)).toBe(25);
		expect(signalHealthStatus(reading)).toBe("flat");
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
});
