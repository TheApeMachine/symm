import { describe, expect, it } from "vitest";

import {
	isOhlcBootstrapRequest,
	isOhlcRow,
	ohlcRowToBar,
	OhlcDataProvider,
} from "#/components/symm/ohlc-data-provider";

describe("isOhlcRow", () => {
	it("accepts hub ohlc.Data payloads", () => {
		expect(
			isOhlcRow({
				symbol: "BTC/EUR",
				open: 50000,
				high: 50100,
				low: 49900,
				close: 50050,
				volume: 12,
				interval_begin: "2026-05-23T12:00:00.000000000Z",
			}),
		).toBe(true);
	});

	it("rejects hello frames", () => {
		expect(isOhlcRow({ event: "hello", ts: "2026-05-23T12:00:00Z" })).toBe(
			false,
		);
	});
});

describe("isOhlcBootstrapRequest", () => {
	it("accepts chart bootstrap payloads", () => {
		expect(
			isOhlcBootstrapRequest({
				symbol: "BTC/EUR",
				interval: 3600,
				startDate: new Date("2026-01-01T00:00:00Z"),
				count: 500,
			}),
		).toBe(true);
	});

	it("rejects hub ohlc rows", () => {
		expect(
			isOhlcBootstrapRequest({
				symbol: "BTC/EUR",
				open: 1,
				high: 2,
				low: 0.5,
				close: 1.5,
			}),
		).toBe(false);
	});
});

describe("ohlcRowToBar", () => {
	it("maps interval_begin to unix seconds", () => {
		const bar = ohlcRowToBar({
			symbol: "BTC/EUR",
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
			interval_begin: "2026-05-23T12:00:00.000Z",
		});

		expect(bar.sec).toBe(
			Math.floor(Date.parse("2026-05-23T12:00:00.000Z") / 1000),
		);
		expect(bar.open).toBe(1);
		expect(bar.volume).toBe(3);
	});
});

describe("OhlcDataProvider.ingest bootstrap", () => {
	it("returns empty arrays when no history exists", () => {
		const data = OhlcDataProvider.ingest({
			symbol: "NOHIST/EUR",
			interval: 3600,
			startDate: new Date("2026-01-01T00:00:00Z"),
			count: 500,
		});

		expect(data).toBeDefined();
		expect(data?.xValues).toEqual([]);
		expect(data?.openValues).toEqual([]);
	});

	it("returns buffered hub history on bootstrap", () => {
		OhlcDataProvider.ingest({
			symbol: "BTC/EUR",
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
			interval_begin: "2026-05-23T12:00:00.000Z",
		});

		const data = OhlcDataProvider.ingest({
			symbol: "BTC/EUR",
			interval: 3600,
			startDate: new Date("2026-01-01T00:00:00Z"),
			count: 500,
		});

		expect(data?.closeValues.at(-1)).toBe(1.5);
	});
});

describe("OhlcDataProvider.ingest hub", () => {
	it("routes parsed rows to registered symbol sinks", () => {
		const received: number[] = [];
		const unregister = OhlcDataProvider.registerSymbol("ETH/EUR", (bar) => {
			received.push(bar.close);
		});

		OhlcDataProvider.ingest({
			symbol: "ETH/EUR",
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
			interval_begin: "2026-05-23T12:00:00.000Z",
		});

		expect(received).toEqual([1.5]);
		unregister();
	});

	it("accepts hub ohlc rows with interval metadata", () => {
		const received: number[] = [];
		const unregister = OhlcDataProvider.registerSymbol("LTC/EUR", (bar) => {
			received.push(bar.close);
		});

		OhlcDataProvider.ingest({
			symbol: "LTC/EUR",
			open: 66366.9,
			high: 66368.1,
			low: 66366.9,
			close: 66368.1,
			volume: 0.0049902,
			interval: 1,
			interval_begin: "2026-05-25T13:54:00Z",
		});

		expect(received).toEqual([66368.1]);
		unregister();
	});

	it("accepts legacy candle_bar ui events", () => {
		const received: number[] = [];
		const unregister = OhlcDataProvider.registerSymbol("SOL/EUR", (bar) => {
			received.push(bar.close);
		});

		OhlcDataProvider.ingest({
			event: "candle_bar",
			symbol: "SOL/EUR",
			sec: 1710000000,
			open: 1,
			high: 2,
			low: 0.5,
			close: 2,
			volume: 4,
		});

		expect(received).toEqual([2]);
		unregister();
	});
});

describe("OhlcDataProvider.getRandomOHLCVData", () => {
	it("returns bootstrap arrays compatible with ExampleDataProvider call sites", () => {
		const startDate = new Date("2026-02-01T00:00:00Z");
		const data = OhlcDataProvider.getRandomOHLCVData(
			500,
			64000,
			startDate,
			3600,
		);

		expect(Array.isArray(data.xValues)).toBe(true);
		expect(data.openValues).toHaveLength(data.xValues.length);
	});
});
