import type { OhlcStore } from "#/components/symm/ohlc-data-provider";

const unregisterFns = new Map<string, () => void>();

export const registerTradeChart = (
	ohlcStore: OhlcStore,
	symbol: string,
	appendBar: (bar: {
		sec: number;
		open: number;
		high: number;
		low: number;
		close: number;
		volume: number;
	}) => void,
) => {
	unregisterTradeChart(symbol);
	unregisterFns.set(symbol, ohlcStore.registerSymbol(symbol, appendBar));

	return () => unregisterTradeChart(symbol);
};

export const unregisterTradeChart = (symbol: string) => {
	unregisterFns.get(symbol)?.();
	unregisterFns.delete(symbol);
};
