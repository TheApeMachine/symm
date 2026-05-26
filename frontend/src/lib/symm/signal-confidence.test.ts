import { describe, expect, it } from "vitest";

import {
	confidenceToGaugePercent,
	formatSignalConfidence,
	isSignalSource,
} from "#/lib/symm/signal-confidence";

describe("isSignalSource", () => {
	it("accepts backend liquidity source for basis gauge", () => {
		expect(isSignalSource("liquidity")).toBe(true);
		expect(isSignalSource("basis")).toBe(false);
	});
});

describe("confidenceToGaugePercent", () => {
	it("maps unit confidence to gauge needle range", () => {
		expect(confidenceToGaugePercent(0)).toBe(0);
		expect(confidenceToGaugePercent(0.42)).toBe(42);
		expect(confidenceToGaugePercent(0.87)).toBe(87);
	});
});

describe("formatSignalConfidence", () => {
	it("formats normalized unit scores as gauge percent labels", () => {
		expect(formatSignalConfidence(0.421)).toBe("42.1");
		expect(formatSignalConfidence(0.873)).toBe("87.3");
	});
});
