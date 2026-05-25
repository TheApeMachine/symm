import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";

type TradeChartHandler = (bar: {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
}) => void;

const handlers = new Map<string, TradeChartHandler>();
const unregisterFns = new Map<string, () => void>();

export const registerTradeChart = (
	symbol: string,
	appendBar: TradeChartHandler,
) => {
	handlers.set(symbol, appendBar);
	unregisterFns.get(symbol)?.();
	unregisterFns.set(symbol, OhlcDataProvider.registerSymbol(symbol, appendBar));
};

export const unregisterTradeChart = (symbol: string) => {
	handlers.delete(symbol);
	unregisterFns.get(symbol)?.();
	unregisterFns.delete(symbol);
};
