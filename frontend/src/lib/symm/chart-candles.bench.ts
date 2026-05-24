import { bench, describe } from "vitest";

import type { PriceTickEvent } from "#/lib/symm/events";
import { aggregateTicksToCandles } from "#/lib/symm/chart-candles";

const ticks: PriceTickEvent[] = Array.from({ length: 360 }, (_, index) => ({
	event: "price_tick",
	ts: `2026-05-23T17:10:${String(index % 60).padStart(2, "0")}.000000000Z`,
	symbol: "ZETA/EUR",
	last: 0.043 + (index % 7) * 0.0001,
	bid: 0.0426,
	ask: 0.0442,
	change_pct_24h: 0,
	at: `2026-05-23T17:10:${String(index % 60).padStart(2, "0")}.000000000Z`,
}));

describe("aggregateTicksToCandles", () => {
	bench("360 ticker rows into 5s candles", () => {
		aggregateTicksToCandles(ticks, 5);
	});
});
