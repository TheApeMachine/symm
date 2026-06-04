import { describe, expect, it } from "vitest";

import {
	confidenceFromGaugePayload,
	formatGaugePayloadValue,
	gaugePayloadEntries,
	gaugeWirePayload,
} from "#/components/charts/confidence/gauge-payload";

describe("gaugeWirePayload", () => {
	it("copies gauge fields and drops wire routing keys", () => {
		expect(
			gaugeWirePayload({
				chart: "gauge",
				event: "tick",
				source: "hawkes",
				confidence: 1.2,
				snr: 0.8,
			}),
		).toEqual({
			source: "hawkes",
			confidence: 1.2,
			snr: 0.8,
		});
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
