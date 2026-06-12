import { describe, expect, it } from "vitest";

import {
	latestCandleTime,
	parseCandleFrame,
} from "#/components/charts/trade/init-trade-chart";

describe("parseCandleFrame", () => {
	it("normalizes valid OHLC frames by symbol", () => {
		const candle = parseCandleFrame({
			symbol: "ltc/usd",
			sec: 1_780_627_020,
			open: 42.1,
			high: 42.3,
			low: 42,
			close: 42.2,
			volume: 14,
		});

		expect(candle).toEqual({
			symbol: "LTC/USD",
			sec: 1_780_627_020,
			open: 42.1,
			high: 42.3,
			low: 42,
			close: 42.2,
			volume: 14,
		});
	});

	it("rejects partial frames", () => {
		expect(
			parseCandleFrame({
				symbol: "LTC/USD",
				sec: 1_780_627_020,
				open: 42.1,
			}),
		).toBeNull();
	});
});

describe("latestCandleTime", () => {
	it("reads the last candle timestamp", () => {
		const ohlc = {
			count: () => 2,
			getNativeXValues: () => ({
				get: (index: number) => [100, 160][index],
			}),
		};

		expect(latestCandleTime(ohlc as never)).toBe(160);
	});

	it("returns null for an empty series", () => {
		const ohlc = {
			count: () => 0,
			getNativeXValues: () => ({
				get: () => 0,
			}),
		};

		expect(latestCandleTime(ohlc as never)).toBeNull();
	});
});
