import { describe, expect, it } from "vitest";

import {
	confidenceToGaugePercent,
	formatSignalConfidence,
} from "#/lib/symm/signal-confidence";

describe("confidenceToGaugePercent", () => {
	it("maps normalized confidence to gauge needle range", () => {
		expect(confidenceToGaugePercent(0)).toBe(0);
		expect(confidenceToGaugePercent(0.42)).toBe(42);
		expect(confidenceToGaugePercent(1)).toBe(100);
	});

	it("rejects confidence above the unit interval", () => {
		expect(() => confidenceToGaugePercent(1.5)).toThrow(/out of unit interval/);
	});
});

describe("formatSignalConfidence", () => {
	it("formats normalized confidence as gauge percent labels", () => {
		expect(formatSignalConfidence(0.421)).toBe("42.1");
		expect(formatSignalConfidence(0.873)).toBe("87.3");
	});
});
