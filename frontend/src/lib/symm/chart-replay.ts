import type {
	ChartSeedEvent,
	PriceTickEvent,
	StatusEvent,
	SymmEvent,
} from "#/lib/symm/events";

export const buildChartReplayEvents = (
	symbol: string,
	seed: ChartSeedEvent | undefined,
	ticks: PriceTickEvent[],
	status: StatusEvent | undefined,
): SymmEvent[] => {
	const events: SymmEvent[] = [];

	if (seed) {
		events.push(seed);
	}

	for (const tick of ticks) {
		if (String(tick.symbol) !== symbol) {
			continue;
		}

		events.push(tick);
	}

	if (status) {
		events.push(status);
	}

	return events;
};
