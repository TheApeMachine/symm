import { describe, expect, it } from "vitest";

import type { CandleBarEvent } from "#/lib/symm/events";
import {
	applyCandleBar,
	applyChartSeed,
	chartStore,
} from "#/lib/symm/stores/chart-store";

describe("chart-store", () => {
	it("appends and updates candle bars per symbol", () => {
		const first: CandleBarEvent = {
			event: "candle_bar",
			ts: "2026-05-24T03:00:00Z",
			symbol: "BTC/EUR",
			sec: 100,
			open: 1,
			high: 1.1,
			low: 0.9,
			close: 1.05,
		};
		const updated: CandleBarEvent = {
			...first,
			close: 1.08,
			high: 1.12,
		};
		const next: CandleBarEvent = {
			...first,
			sec: 105,
			open: 1.08,
			high: 1.15,
			low: 1.07,
			close: 1.14,
		};

		applyCandleBar(first);
		applyCandleBar(updated);
		applyCandleBar(next);

		const state = chartStore.state.symbols["BTC/EUR"];

		expect(state?.candles).toHaveLength(2);
		expect(state?.candles[0]?.close).toBe(1.08);
		expect(state?.candles[1]?.sec).toBe(105);
		expect(state?.latestPrice).toBe(1.14);
	});

	it("seeds historical bars from chart_seed", () => {
		applyChartSeed({
			event: "chart_seed",
			ts: "2026-05-24T03:00:00Z",
			symbol: "ETH/EUR",
			bars: [
				{ t: 10, o: 2, h: 2.1, l: 1.9, c: 2.05 },
				{ t: 15, o: 2.05, h: 2.2, l: 2, c: 2.15 },
			],
		});

		const state = chartStore.state.symbols["ETH/EUR"];

		expect(state?.candles).toHaveLength(2);
		expect(state?.candles[1]?.close).toBe(2.15);
	});
});
