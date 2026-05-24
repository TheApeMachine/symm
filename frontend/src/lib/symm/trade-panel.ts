import type {
	StatusEvent,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";
import { sortOpenPositions } from "#/lib/symm/positions";

export type TradePanelRow = {
	key: string;
	kind: "open" | "enter" | "exit";
	symbol: string;
	regime: string;
	reason: string;
	pnl_eur?: number;
};

export const selectTradePanelRows = (input: {
	status?: StatusEvent;
	trades: Array<TradeEnterEvent | TradeExitEvent>;
}): TradePanelRow[] => {
	const open = sortOpenPositions(input.status?.positions ?? []);
	const rows: TradePanelRow[] = open.map((position) => ({
		key: `open:${position.symbol}`,
		kind: "open",
		symbol: position.symbol,
		regime: position.regime,
		reason: "open",
	}));

	const exits = input.trades.filter(
		(trade): trade is TradeExitEvent => trade.event === "trade_exit",
	);

	for (const trade of [...exits].reverse()) {
		rows.push({
			key: `${trade.event}:${trade.ts}:${trade.symbol}`,
			kind: "exit",
			symbol: trade.symbol,
			regime: trade.regime,
			reason: trade.reason,
			pnl_eur: trade.pnl_eur,
		});
	}

	return rows;
};
