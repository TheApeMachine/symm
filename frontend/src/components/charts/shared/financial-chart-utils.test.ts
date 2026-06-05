import { NumberRange } from "scichart";
import { describe, expect, it } from "vitest";

import {
	candleChartXExtents,
	isViewportFollowingLiveEdge,
	priceLabelDecimals,
	shiftTrailingVisibleRange,
	visibleRangesMatch,
} from "#/components/charts/shared/financial-chart-utils";

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

describe("isViewportFollowingLiveEdge", () => {
	it("treats a trailing viewport as following live data", () => {
		const priorLastX = 1_780_627_020;
		const range = new NumberRange(priorLastX - 120, priorLastX + 30);

		expect(isViewportFollowingLiveEdge(range, priorLastX, 60)).toBe(true);
	});

	it("treats a historical viewport as user-controlled", () => {
		const priorLastX = 1_780_627_020;
		const range = new NumberRange(priorLastX - 600_000, priorLastX - 300_000);

		expect(isViewportFollowingLiveEdge(range, priorLastX, 60)).toBe(false);
	});
});

describe("visibleRangesMatch", () => {
	it("matches programmatic viewport updates within epsilon", () => {
		const left = new NumberRange(1000, 2000);
		const right = new NumberRange(1000.5, 2000.5);

		expect(visibleRangesMatch(left, right)).toBe(true);
	});

	it("detects user viewport changes", () => {
		const left = new NumberRange(1000, 2000);
		const right = new NumberRange(1500, 2500);

		expect(visibleRangesMatch(left, right)).toBe(false);
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
