import { createStore } from "@tanstack/react-store";

import type {
	CandleBarEvent,
	ChartSeedEvent,
	StatusPosition,
} from "#/lib/symm/events";

export type SymbolChartState = {
	candles: CandleBarEvent[];
	latestPrice: number;
	position?: StatusPosition;
};

export type ChartStoreState = {
	symbols: Record<string, SymbolChartState>;
};

const emptySymbolState = (): SymbolChartState => ({
	candles: [],
	latestPrice: 0,
});

export const chartStore = createStore<ChartStoreState>({
	symbols: {},
});

const upsertCandle = (
	candles: CandleBarEvent[],
	bar: CandleBarEvent,
): CandleBarEvent[] => {
	const last = candles[candles.length - 1];

	if (last?.sec === bar.sec) {
		return [...candles.slice(0, -1), bar];
	}

	if (last && last.sec > bar.sec) {
		return candles;
	}

	return [...candles, bar];
};

export const applyChartSeed = (seed: ChartSeedEvent): void => {
	const symbol = String(seed.symbol);
	const bars =
		seed.bars
			?.filter((bar) => bar.t > 0 && bar.c > 0)
			.map(
				(bar) =>
					({
						event: "candle_bar",
						ts: seed.ts,
						symbol,
						sec: bar.t,
						open: bar.o,
						high: bar.h,
						low: bar.l,
						close: bar.c,
					}) satisfies CandleBarEvent,
			) ?? [];

	chartStore.setState((state) => ({
		symbols: {
			...state.symbols,
			[symbol]: {
				candles: bars,
				latestPrice: bars.at(-1)?.close ?? 0,
				position: state.symbols[symbol]?.position,
			},
		},
	}));
};

export const applyCandleBar = (bar: CandleBarEvent): void => {
	const symbol = String(bar.symbol);

	chartStore.setState((state) => {
		const previous = state.symbols[symbol] ?? emptySymbolState();

		return {
			symbols: {
				...state.symbols,
				[symbol]: {
					...previous,
					candles: upsertCandle(previous.candles, bar),
					latestPrice: bar.close,
				},
			},
		};
	});
};

export const applyChartPosition = (
	symbol: string,
	position: StatusPosition | undefined,
): void => {
	chartStore.setState((state) => {
		const previous = state.symbols[symbol] ?? emptySymbolState();

		return {
			symbols: {
				...state.symbols,
				[symbol]: {
					...previous,
					position,
				},
			},
		};
	});
};

export const symbolChartState = (
	state: ChartStoreState,
	symbol: string,
): SymbolChartState => state.symbols[symbol] ?? emptySymbolState();
