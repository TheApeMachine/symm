import { describe, expect, it } from "vitest";

import type {
	ChartSeedEvent,
	PriceTickEvent,
	StatusEvent,
} from "#/lib/symm/events";
import { buildChartReplayEvents } from "#/lib/symm/chart-replay";

describe("buildChartReplayEvents", () => {
	it("filters replay ticks to the requested symbol", () => {
		const seed: ChartSeedEvent = {
			event: "chart_seed",
			ts: "2026-05-23T12:00:00.000000000Z",
			symbol: "BTC/EUR",
			bars: [],
		};
		const ticks: PriceTickEvent[] = [
			{
				event: "price_tick",
				ts: "2026-05-23T12:00:01.000000000Z",
				symbol: "BTC/EUR",
				last: 1.05,
				bid: 1.04,
				ask: 1.06,
				at: "2026-05-23T12:00:01.000000000Z",
			},
			{
				event: "price_tick",
				ts: "2026-05-23T12:00:02.000000000Z",
				symbol: "ETH/EUR",
				last: 2.05,
				bid: 2.04,
				ask: 2.06,
				at: "2026-05-23T12:00:02.000000000Z",
			},
		];

		const events = buildChartReplayEvents("BTC/EUR", seed, ticks, undefined);

		expect(events.map((event) => event.event)).toEqual([
			"chart_seed",
			"price_tick",
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
