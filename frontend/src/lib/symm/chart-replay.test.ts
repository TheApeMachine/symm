import { describe, expect, it } from "vitest";

import type {
	CandleBarEvent,
	ChartSeedEvent,
	StatusEvent,
} from "#/lib/symm/events";
import { buildChartReplayEvents } from "#/lib/symm/chart-replay";

describe("buildChartReplayEvents", () => {
	it("filters replay candles to the requested symbol", () => {
		const seed: ChartSeedEvent = {
			event: "chart_seed",
			ts: "2026-05-23T12:00:00.000000000Z",
			symbol: "BTC/EUR",
			bars: [],
		};
		const candles: CandleBarEvent[] = [
			{
				event: "candle_bar",
				ts: "2026-05-23T12:00:01.000000000Z",
				symbol: "BTC/EUR",
				sec: 1_746_000_000,
				open: 1,
				high: 1.1,
				low: 0.9,
				close: 1.05,
			},
			{
				event: "candle_bar",
				ts: "2026-05-23T12:00:02.000000000Z",
				symbol: "ETH/EUR",
				sec: 1_746_000_005,
				open: 2,
				high: 2.1,
				low: 1.9,
				close: 2.05,
			},
		];

		const events = buildChartReplayEvents("BTC/EUR", seed, candles, undefined);

		expect(events.map((event) => event.event)).toEqual([
			"chart_seed",
			"candle_bar",
		]);
	});
});

describe("buildChartReplayEvents status tail", () => {
	it("appends status after seed and replay", () => {
		const status: StatusEvent = {
			event: "status",
			ts: "2026-05-23T12:00:03.000000000Z",
			equity_eur: 1000,
			cash_eur: 1000,
			closed_pnl_eur: 0,
			trade_count: 0,
			win_rate: 0,
			open_count: 0,
			positions: [],
		};

		const events = buildChartReplayEvents("BTC/EUR", undefined, [], status);

		expect(events.map((event) => event.event)).toEqual(["status"]);
	});
});
