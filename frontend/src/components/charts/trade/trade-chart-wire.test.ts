import { describe, expect, it } from "vitest";

import {
	ingestCandleWire,
	registerTradeChart,
} from "#/components/charts/trade/trade-chart-wire";

describe("ingestCandleWire", () => {
	it("routes backend ohlc rows to registered chart sinks", () => {
		const received: number[] = [];
		const unregister = registerTradeChart("ETH/EUR", (bar) => {
			received.push(bar.close);
		});

		ingestCandleWire({
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

	it("buffers bars until a chart registers", () => {
		ingestCandleWire({
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

		expect(received).toEqual([1.5]);
		unregister();
	});

	it("rejects rows without sec", () => {
		const received: number[] = [];
		const unregister = registerTradeChart("BTC/EUR", (bar) => {
			received.push(bar.close);
		});

		ingestCandleWire({
			symbol: "BTC/EUR",
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
			interval_begin: "2026-05-25T13:54:00Z",
		});

		expect(received).toEqual([]);
		unregister();
	});
});
