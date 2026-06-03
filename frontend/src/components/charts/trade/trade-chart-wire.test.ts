import { describe, expect, it } from "vitest";

import {
	ingestCandleWire,
	parseCandleWire,
	registerTradeChart,
} from "#/components/charts/trade/trade-chart-wire";

describe("parseCandleWire", () => {
	it("accepts candle_bar ui events", () => {
		expect(
			parseCandleWire({
				event: "candle_bar",
				symbol: "SOL/EUR",
				sec: 1_710_000_000,
				open: 1,
				high: 2,
				low: 0.5,
				close: 2,
				volume: 4,
			}),
		).toEqual({
			symbol: "SOL/EUR",
			bar: {
				sec: 1_710_000_000,
				open: 1,
				high: 2,
				low: 0.5,
				close: 2,
				volume: 4,
			},
		});
	});

	it("accepts hub ohlc rows with interval metadata", () => {
		const parsed = parseCandleWire({
			symbol: "LTC/EUR",
			open: 66366.9,
			high: 66368.1,
			low: 66366.9,
			close: 66368.1,
			volume: 0.0049902,
			interval: 1,
			interval_begin: "2026-05-25T13:54:00Z",
		});

		expect(parsed?.symbol).toBe("LTC/EUR");
		expect(parsed?.bar.close).toBe(66368.1);
		expect(parsed?.bar.sec).toBe(
			Math.floor(Date.parse("2026-05-25T13:54:00Z") / 1000),
		);
	});

	it("rejects mark events so ticks do not become candles", () => {
		expect(
			parseCandleWire({
				event: "mark",
				ts: "2026-05-28T01:10:10Z",
				symbol: "MARKLESS/EUR",
				price: 0.42,
			}),
		).toBeUndefined();
	});

	it("rejects hello frames", () => {
		expect(
			parseCandleWire({ event: "hello", ts: "2026-05-23T12:00:00Z" }),
		).toBeUndefined();
	});
});

describe("ingestCandleWire", () => {
	it("routes parsed rows to registered chart sinks", () => {
		const received: number[] = [];
		const unregister = registerTradeChart("ETH/EUR", (bar) => {
			received.push(bar.close);
		});

		ingestCandleWire({
			event: "candle_bar",
			symbol: "ETH/EUR",
			sec: 1_710_000_000,
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
		});

		expect(received).toEqual([1.5]);
		unregister();
	});

	it("does not buffer bars before a chart registers", () => {
		ingestCandleWire({
			event: "candle_bar",
			symbol: "BTC/EUR",
			sec: 1_710_000_000,
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
		});

		const received: number[] = [];
		const unregister = registerTradeChart("BTC/EUR", (bar) => {
			received.push(bar.close);
		});

		expect(received).toEqual([]);
		unregister();
	});
});
