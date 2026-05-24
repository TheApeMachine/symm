import { describe, expect, it } from "vitest";

import type { PriceTickEvent } from "#/lib/symm/events";
import {
	aggregateTicksToCandles,
	bucketSecond,
	widenFlatOHLC,
} from "#/lib/symm/chart-candles";

function tick(
	symbol: string,
	last: number,
	at: string,
	bid = last,
	ask = last,
): PriceTickEvent {
	return {
		event: "price_tick",
		ts: at,
		symbol,
		last,
		bid,
		ask,
		change_pct_24h: 0,
		at,
	};
}

describe("aggregateTicksToCandles", () => {
	it("merges ticks in the same bucket using last trade only", () => {
		const bars = aggregateTicksToCandles(
			[
				tick(
					"ZETA/EUR",
					0.043,
					"2026-05-23T17:18:01.000000000Z",
					0.0426,
					0.0442,
				),
				tick(
					"ZETA/EUR",
					0.0434,
					"2026-05-23T17:18:02.000000000Z",
					0.0426,
					0.0442,
				),
				tick(
					"ZETA/EUR",
					0.0431,
					"2026-05-23T17:18:06.000000000Z",
					0.0426,
					0.0442,
				),
			],
			5,
		);

		expect(bars).toHaveLength(2);
		expect(bars[0].open).toBe(0.043);
		expect(bars[0].close).toBe(0.0434);
		expect(bars[0].high).toBe(0.0434);
		expect(bars[0].low).toBe(0.043);
		expect(bars[1].open).toBe(0.0431);
	});

	it("does not widen candles to the bid/ask spread", () => {
		const bars = aggregateTicksToCandles(
			[tick("ZETA/EUR", 0.043, "2026-05-23T17:18:01.000000000Z", 0.01, 0.09)],
			5,
		);

		expect(bars[0].high).toBe(0.043);
		expect(bars[0].low).toBe(0.043);
	});
});

describe("bucketSecond", () => {
	it("aligns timestamps to candle buckets", () => {
		expect(bucketSecond(171, 5)).toBe(170);
	});
});

describe("widenFlatOHLC", () => {
	it("adds a visible body for flat prices", () => {
		const bar = widenFlatOHLC(0.043, 0.043, 0.043, 0.043);

		expect(bar.high).toBeGreaterThan(0.043);
		expect(bar.low).toBeLessThan(0.043);
	});
});
