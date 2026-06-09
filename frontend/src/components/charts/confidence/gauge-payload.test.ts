import { describe, expect, it } from "vitest";

import {
	confidenceFromGaugePayload,
	formatGaugePayloadValue,
	gaugePayloadEntries,
	gaugeWarmupPercent,
	surpriseFromGaugePayload,
	surpriseScaleMax,
	surpriseThresholdFromGaugePayload,
} from "#/components/charts/confidence/gauge-payload";

describe("surpriseFromGaugePayload", () => {
	it("reads surprise for the linear gauge", () => {
		expect(surpriseFromGaugePayload({ surprise: 2.5 })).toBe(2.5);
	});

	it("falls back to snr for older frames", () => {
		expect(surpriseFromGaugePayload({ snr: 1.75 })).toBe(1.75);
	});

	it("returns zero when surprise is missing", () => {
		expect(surpriseFromGaugePayload({ confidence: 1 })).toBe(0);
	});
});

describe("surpriseThresholdFromGaugePayload", () => {
	it("reads the configured threshold", () => {
		expect(surpriseThresholdFromGaugePayload({ surprise_threshold: 2 })).toBe(
			2,
		);
	});

	it("returns null when threshold is missing", () => {
		expect(surpriseThresholdFromGaugePayload({ surprise: 1 })).toBeNull();
	});
});

describe("surpriseScaleMax", () => {
	it("uses three thresholds as a fixed range", () => {
		expect(surpriseScaleMax({ surprise_threshold: 2 }, 1.5)).toBe(6);
		expect(surpriseScaleMax({ surprise_threshold: 2 }, 6)).toBe(6);
	});

	it("defaults to six when threshold is missing", () => {
		expect(surpriseScaleMax({}, 3)).toBe(6);
	});
});

describe("confidenceFromGaugePayload", () => {
	it("reads confidence for the needle", () => {
		expect(confidenceFromGaugePayload({ confidence: 1.5 })).toBe(1.5);
	});

	it("returns zero when confidence is missing", () => {
		expect(confidenceFromGaugePayload({ snr: 2 })).toBe(0);
	});
});

describe("gaugeWarmupPercent", () => {
	it("returns sample progress while calibrating", () => {
		expect(
			gaugeWarmupPercent({
				calibrating: true,
				samples: 30,
				min_samples: 100,
			}),
		).toBe(30);
	});

	it("returns null once calibrated", () => {
		expect(
			gaugeWarmupPercent({
				calibrated: true,
				samples: 100,
				min_samples: 100,
			}),
		).toBeNull();
	});

	it("returns zero before the first warmup frame", () => {
		expect(gaugeWarmupPercent({})).toBe(0);
	});
});

describe("gaugePayloadEntries", () => {
	it("lists sorted keys with formatted values", () => {
		expect(
			gaugePayloadEntries({
				snr: 0.81234,
				confidence: 1,
				source: "fluid",
				factors: [{ name: "div", value: 0.2 }],
			}),
		).toEqual([
			["confidence", "1"],
			["factors", '[{"name":"div","value":0.2}]'],
			["snr", "0.8123"],
			["source", "fluid"],
		]);
	});
});

describe("formatGaugePayloadValue", () => {
	it("formats booleans and arrays", () => {
		expect(formatGaugePayloadValue(true)).toBe("true");
		expect(formatGaugePayloadValue([1, 2])).toBe("[1,2]");
	});
});
