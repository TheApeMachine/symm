import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";

const unregisterFns = new Map<string, () => void>();

export const registerTradeChart = (
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
	unregisterFns.get(symbol)?.();
	unregisterFns.set(symbol, OhlcDataProvider.registerSymbol(symbol, appendBar));
};

export const unregisterTradeChart = (symbol: string) => {
	unregisterFns.get(symbol)?.();
	unregisterFns.delete(symbol);
};
