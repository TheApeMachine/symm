import type { PriceTickEvent } from "#/lib/symm/events";
import { tickTimeSec } from "#/lib/symm/events";

export type CandleBar = {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
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
			volume: 0,
		});
	}

	return bars;
}
