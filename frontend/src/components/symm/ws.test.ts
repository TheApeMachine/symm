import { afterEach, describe, expect, it } from "vitest";

import type { OhlcBar } from "#/components/symm/ohlc-data-provider";
import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
import { registerTradeChart } from "#/components/symm/ws";

describe("registerTradeChart", () => {
	afterEach(() => {
		OhlcDataProvider.ingest({
			symbol: "BTC/EUR",
			open: 0,
			high: 0,
			low: 0,
			close: 0,
			volume: 0,
		});
	});

	it("routes ingested candle bars to registered charts", () => {
		const bars: OhlcBar[] = [];

		registerTradeChart("BTC/EUR", (bar) => {
			bars.push(bar);
		});

		OhlcDataProvider.ingest({
			event: "candle_bar",
			symbol: "BTC/EUR",
			sec: 1_700_000_000,
			open: 1,
			high: 2,
			low: 0.5,
			close: 1.5,
			volume: 3,
		});

		expect(bars).toHaveLength(1);
		expect(bars[0]?.close).toBe(1.5);
	});
});
