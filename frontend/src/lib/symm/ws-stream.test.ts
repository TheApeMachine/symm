import { describe, expect, it } from "vitest";

import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
import { TradesDataProvider } from "#/components/symm/trades-data-provider";
import { routePayload } from "#/lib/symm/ws-stream";
import { TickStore } from "#/lib/symm/tick-store";

describe("routePayload", () => {
	it("increments tick count from crypto tick events", () => {
		TickStore.reset();

		routePayload({
			event: "tick",
			ts: "2026-05-28T01:10:10Z",
		});
		routePayload({
			event: "tick",
			ts: "2026-05-28T01:10:11Z",
		});

		expect(TickStore.snapshot()).toBe(2);
		TickStore.reset();
	});

	it("updates tick count from heartbeat proof of life events", () => {
		TickStore.reset();

		routePayload({
			event: "heartbeat",
			ts: "2026-05-28T01:10:10Z",
			seq: 42,
			throttled: true,
			queue_depth: 4,
			dropped: 2,
		});

		expect(TickStore.snapshot()).toBe(42);
		expect(TickStore.statusSnapshot()).toEqual({
			seq: 42,
			throttled: true,
			queueDepth: 4,
			dropped: 2,
		});
		TickStore.reset();
	});

	it("routes mark events to trades without mutating candle history", () => {
		TradesDataProvider.reset();
		const bars: number[] = [];
		const unregister = OhlcDataProvider.registerSymbol("ROUTE/EUR", (bar) => {
			bars.push(bar.sec);
		});

		routePayload({
			Type: "paper",
			Currency: "EUR",
			Balance: 198.9,
			Inventory: { ROUTE: 10 },
			AvgEntry: { ROUTE: 0.4 },
			Marks: { "ROUTE/EUR": 0.4 },
		});

		routePayload({
			event: "mark",
			ts: "2026-05-28T01:10:10Z",
			symbol: "ROUTE/EUR",
			price: 0.42,
		});

		expect(bars).toEqual([]);
		expect(TradesDataProvider.snapshot()[0]?.markPrice).toBe(0.42);
		TradesDataProvider.reset();
		unregister();
	});
});
