import { describe, expect, it } from "vitest";

import {
	confidenceFromGaugePayload,
	formatGaugePayloadValue,
	gaugePayloadEntries,
	gaugeWarmupPercent,
} from "#/components/charts/confidence/gauge-payload";

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
