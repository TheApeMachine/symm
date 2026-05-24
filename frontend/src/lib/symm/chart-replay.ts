import type { PriceTickEvent, StatusEvent, SymmEvent } from "#/lib/symm/events";

export const buildChartReplayEvents = (
	symbol: string,
	seed: SymmEvent | undefined,
	ticks: PriceTickEvent[],
	status: StatusEvent | undefined,
): SymmEvent[] => {
	const events: SymmEvent[] = [];

	if (seed) {
		events.push(seed);
	}

	const symbolTicks = ticks.filter((tick) => String(tick.symbol) === symbol);
	if (symbolTicks.length > 0) {
		events.push({
			event: "chart_replay",
			ts: symbolTicks[symbolTicks.length - 1]?.ts ?? "",
			symbol,
			ticks: symbolTicks,
		});
	}

	if (status) {
		events.push(status);
	}

	return events;
};
