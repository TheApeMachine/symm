import { describe, expect, it } from "vitest";

import {
	candleChartXExtents,
	priceLabelDecimals,
	shiftTrailingVisibleRange,
} from "#/components/symm/financial-chart-utils";

describe("candleChartXExtents", () => {
	it("pads a single live bar so candles have horizontal width", () => {
		const sec = 1_747_000_000;
		const { min, max } = candleChartXExtents(sec, sec, 1);

		expect(min).toBeLessThan(sec);
		expect(max).toBeGreaterThan(sec);
		expect(max - min).toBeGreaterThan(0);
	});
});

describe("shiftTrailingVisibleRange", () => {
	it("preserves zoom span while keeping the latest bar in view", () => {
		const shifted = shiftTrailingVisibleRange(1000, 1060, 1200, 60);

		expect(shifted.max - shifted.min).toBe(60);
		expect(shifted.max).toBeGreaterThan(1200);
	});
});

describe("priceLabelDecimals", () => {
	it("uses finer precision for narrow live price spans", () => {
		expect(priceLabelDecimals(0.05)).toBe(4);
		expect(priceLabelDecimals(5)).toBe(3);
		expect(priceLabelDecimals(500)).toBe(1);
		expect(priceLabelDecimals(5000)).toBe(0);
	});
});
