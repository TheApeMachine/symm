import { describe, expect, it } from "vitest";

import { isOhlcRow, ohlcRowToBar } from "#/components/symm/ohlc-data-provider";

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

		expect(bar.sec).toBe(Math.floor(Date.parse("2026-05-23T12:00:00.000Z") / 1000));
		expect(bar.open).toBe(1);
		expect(bar.volume).toBe(3);
	});
});
