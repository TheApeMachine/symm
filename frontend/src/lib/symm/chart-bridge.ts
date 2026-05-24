import type { CandleBarEvent, StatusPosition } from "#/lib/symm/events";

export type TradeChartBridge = {
	appendCandle: (bar: CandleBarEvent) => void;
	seedCandles: (bars: CandleBarEvent[]) => void;
	setPosition: (position: StatusPosition) => void;
	clearPosition: () => void;
	ratchetStop: (stop: number) => void;
	dispose: () => void;
};

const bridges = new Map<string, TradeChartBridge>();

export const registerTradeChartBridge = (
	symbol: string,
	bridge: TradeChartBridge,
): void => {
	bridges.set(symbol, bridge);
};

export const unregisterTradeChartBridge = (symbol: string): void => {
	bridges.delete(symbol);
};

export const tradeChartBridge = (
	symbol: string,
): TradeChartBridge | undefined => bridges.get(symbol);
