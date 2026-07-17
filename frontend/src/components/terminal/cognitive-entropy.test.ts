import { describe, expect, it } from "vitest";
import {
	formatBeamSequence,
	formatEntropyGate,
	isUngatedThreshold,
} from "./cognitive-entropy";

describe("isUngatedThreshold", () => {
	it("treats MaxFloat64 and positive infinity as ungated", () => {
		expect(isUngatedThreshold(Number.MAX_VALUE)).toBe(true);
		expect(isUngatedThreshold(1.7976931348623157e308)).toBe(true);
		expect(isUngatedThreshold(Number.POSITIVE_INFINITY)).toBe(true);
		expect(isUngatedThreshold(1.5)).toBe(false);
		expect(isUngatedThreshold(Number.NaN)).toBe(false);
		expect(isUngatedThreshold(Number.NEGATIVE_INFINITY)).toBe(false);
	});
});

describe("formatEntropyGate", () => {
	it("does not dump MaxFloat64 into the meter label", () => {
		const gate = formatEntropyGate(0, Number.MAX_VALUE);

		expect(gate.ungated).toBe(true);
		expect(gate.value).toBe("0.00 / ungated");
		expect(gate.percent).toBe(0);
	});

	it("formats a real threshold ratio", () => {
		const gate = formatEntropyGate(0.5, 1);

		expect(gate.ungated).toBe(false);
		expect(gate.value).toBe("0.50 / 1.00 bits");
		expect(gate.percent).toBe(50);
	});
});

describe("formatBeamSequence", () => {
	it("tokenizes and truncates long DMT sequences", () => {
		const sequence = Array.from(
			{ length: 12 },
			(_, index) => `tok${index}`,
		).join("_");
		const formatted = formatBeamSequence(sequence, 4);

		expect(formatted.title).toBe(sequence);
		expect(formatted.preview.startsWith("… · ")).toBe(true);
		expect(formatted.preview.includes("tok11")).toBe(true);
		expect(formatted.preview.includes("tok0")).toBe(false);
	});
});
