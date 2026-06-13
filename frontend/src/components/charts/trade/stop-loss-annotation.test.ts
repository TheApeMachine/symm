import { describe, expect, it } from "vitest";

import {
	stopLossAnnotationPoints,
	stopLossOverlayFromPosition,
} from "#/components/charts/trade/stop-loss-annotation";

describe("stopLossOverlayFromPosition", () => {
	it("returns overlay when stop and entry are present", () => {
		expect(
			stopLossOverlayFromPosition({
				avgEntry: 100,
				stopPrice: 95,
			}),
		).toEqual({
			avgEntry: 100,
			stopPrice: 95,
		});
	});

	it("returns null when stop is missing", () => {
		expect(
			stopLossOverlayFromPosition({
				avgEntry: 100,
			}),
		).toBeNull();
	});
});

describe("stopLossAnnotationPoints", () => {
	it("spans the loaded candle range from entry to stop", () => {
		const ohlc = {
			count: () => 3,
			getNativeXValues: () => ({
				get: (index: number) => [1_000, 1_060, 1_120][index],
			}),
		};

		expect(
			stopLossAnnotationPoints(ohlc as never, {
				avgEntry: 42.5,
				stopPrice: 41.8,
			}),
		).toEqual({
			x0: 1_000,
			y0: 42.5,
			x1: 1_120,
			y1: 41.8,
		});
	});

	it("returns null before candles arrive", () => {
		const ohlc = {
			count: () => 0,
			getNativeXValues: () => ({
				get: () => 0,
			}),
		};

		expect(
			stopLossAnnotationPoints(ohlc as never, {
				avgEntry: 42.5,
				stopPrice: 41.8,
			}),
		).toBeNull();
	});
});
