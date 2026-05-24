import type { PriceTickEvent } from "#/lib/symm/events";
import { tickTimeSec } from "#/lib/symm/events";

export type CandleBar = {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
};

export function bucketSecond(sec: number, candleSeconds: number): number {
	return Math.floor(sec / candleSeconds) * candleSeconds;
}

export function aggregateTicksToCandles(
	ticks: PriceTickEvent[],
	candleSeconds: number,
): CandleBar[] {
	const bars: CandleBar[] = [];

	for (const tick of ticks) {
		if (tick.last <= 0) {
			continue;
		}

		const bucketSec = bucketSecond(tickTimeSec(tick), candleSeconds);
		const lastBar = bars[bars.length - 1];

		if (lastBar && lastBar.sec === bucketSec) {
			lastBar.high = Math.max(lastBar.high, tick.last);
			lastBar.low = Math.min(lastBar.low, tick.last);
			lastBar.close = tick.last;
			continue;
		}

		bars.push({
			sec: bucketSec,
			open: tick.last,
			high: tick.last,
			low: tick.last,
			close: tick.last,
		});
	}

	return bars;
}

export function widenFlatOHLC(
	open: number,
	high: number,
	low: number,
	close: number,
): { open: number; high: number; low: number; close: number } {
	const mid = close || open || high || low;

	if (mid <= 0) {
		return { open, high, low, close };
	}

	const minSpread = Math.max(Math.abs(mid) * 1e-5, 1e-8);

	if (high - low >= minSpread) {
		return { open, high, low, close };
	}

	const half = minSpread / 2;

	return {
		open,
		high: Math.max(high, mid + half),
		low: Math.min(low, mid - half),
		close,
	};
}
