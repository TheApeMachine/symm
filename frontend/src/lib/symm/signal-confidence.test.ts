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
	it("maps SNR confidence to gauge needle range", () => {
		expect(confidenceToGaugePercent(0)).toBe(0);
		expect(confidenceToGaugePercent(0.42)).toBe(10.5);
		expect(confidenceToGaugePercent(4)).toBe(100);
	});
});

describe("formatSignalConfidence", () => {
	it("formats SNR scores as gauge percent labels", () => {
		expect(formatSignalConfidence(0.421)).toBe("10.5");
		expect(formatSignalConfidence(3.492)).toBe("87.3");
	});
});
