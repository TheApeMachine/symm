import type { CandleBarEvent, StatusEvent, SymmEvent } from "#/lib/symm/events";

export const buildChartReplayEvents = (
	symbol: string,
	seed: SymmEvent | undefined,
	candles: CandleBarEvent[],
	status: StatusEvent | undefined,
): SymmEvent[] => {
	const events: SymmEvent[] = [];

	if (seed) {
		events.push(seed);
	}

	for (const bar of candles) {
		if (String(bar.symbol) !== symbol) {
			continue;
		}

		events.push(bar);
	}

	if (status) {
		events.push(status);
	}

	return events;
};
