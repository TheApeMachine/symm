import { createStore } from "@tanstack/react-store";

import type {
	ScoreboardEvent,
	StatusEvent,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";

const MAX_TRADES = 40;

export type StatusStoreState = {
	status?: StatusEvent;
	scoreboard?: ScoreboardEvent;
	trades: Array<TradeEnterEvent | TradeExitEvent>;
};

export const statusStore = createStore<StatusStoreState>({
	trades: [],
});

const tradeKey = (trade: TradeEnterEvent | TradeExitEvent) =>
	`${trade.event}:${trade.ts}:${trade.symbol}`;

export const pruneClosedTradeEnters = (
	trades: Array<TradeEnterEvent | TradeExitEvent>,
	openSymbols: ReadonlySet<string>,
): Array<TradeEnterEvent | TradeExitEvent> =>
	trades.filter(
		(trade) => trade.event !== "trade_enter" || openSymbols.has(trade.symbol),
	);

export const appendTrade = (trade: TradeEnterEvent | TradeExitEvent): void => {
	statusStore.setState((state) => {
		const key = tradeKey(trade);
		if (state.trades.some((row) => tradeKey(row) === key)) {
			return state;
		}

		return {
			...state,
			trades: [trade, ...state.trades].slice(0, MAX_TRADES),
		};
	});
};

export const applyStatus = (status: StatusEvent): void => {
	const openSymbols = new Set(
		status.positions?.map((position) => position.symbol) ?? [],
	);

	statusStore.setState((state) => ({
		...state,
		status,
		trades: pruneClosedTradeEnters(state.trades, openSymbols),
	}));
};

export const applyScoreboard = (scoreboard: ScoreboardEvent): void => {
	statusStore.setState((state) => ({
		...state,
		scoreboard,
	}));
};
