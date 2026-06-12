import { describe, expect, it } from "vitest";

import {
	confidenceMeterValue,
	healthMeterValue,
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
		expect(reading?.calibrated).toBe(true);
	});
});

describe("signal diagnostics meters", () => {
	const calibratedReading = {
		source: "hawkes",
		confidence: 0.6,
		surprise: 3,
		surpriseThreshold: 2,
		samples: 0,
		minSamples: 0,
		calibrating: false,
		calibrated: true,
		updatedAt: Date.now(),
	};

	it("computes confidence and surprise meters", () => {
		expect(confidenceMeterValue(calibratedReading)).toBe(60);
		expect(surpriseMeterValue(calibratedReading)).toBe(50);
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
});
