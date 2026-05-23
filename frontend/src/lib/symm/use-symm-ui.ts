import { useEffect, useRef, useSyncExternalStore } from "react";

import type {
	DecisionTraceEvent,
	EnginePulseEvent,
	FieldSnapshotEvent,
	ScoreboardEvent,
	StatusEvent,
	StatusPosition,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";
import { pickMarketWatchSymbol } from "#/lib/symm/events";
import type { EvaluationState } from "#/lib/symm/evaluation-store";
import { rankedEvaluations } from "#/lib/symm/evaluation-store";
import {
	arrayEqual,
	getUIState,
	selectPositionSymbols,
	startSymmFeed,
	subscribeUISelector,
	type SymmUIState,
} from "#/lib/symm/feed";

function useSymmSelector<T>(
	selector: (state: SymmUIState) => T,
	equals: (a: T, b: T) => boolean = Object.is,
): T {
	const snapshotRef = useRef<T>(selector(getUIState()));

	return useSyncExternalStore(
		(onStoreChange) =>
			subscribeUISelector(selector, equals, () => {
				const next = selector(getUIState());
				if (!equals(snapshotRef.current, next)) {
					snapshotRef.current = next;
					onStoreChange();
				}
			}),
		() => snapshotRef.current,
		() => snapshotRef.current,
	);
}

export function useSymmFeed(url?: string): void {
	useEffect(() => {
		startSymmFeed(url);
	}, [url]);
}

export function useSymmConnected(): boolean {
	return useSymmSelector((state) => state.connected);
}

export function useSymmStatus(): StatusEvent | undefined {
	return useSymmSelector((state) => state.status);
}

export function useSymmPositionSymbols(): string[] {
	return useSymmSelector(selectPositionSymbols, arrayEqual);
}

export function useSymmScoreboard(): ScoreboardEvent | undefined {
	return useSymmSelector((state) => state.scoreboard);
}

export function useSymmDecisionTrace(): DecisionTraceEvent | undefined {
	return useSymmSelector((state) => state.decisionTrace);
}

export function useSymmFieldSnapshot(): FieldSnapshotEvent | undefined {
	return useSymmSelector((state) => state.fieldSnapshot);
}

export function useSymmEnginePulse(): EnginePulseEvent | undefined {
	return useSymmSelector((state) => state.enginePulse);
}

export function useSymmEvaluation(): EvaluationState {
	return useSymmSelector(
		(state) => state.evaluation,
		(left, right) => left === right,
	);
}

export function useSymmRankedEvaluations() {
	return useSymmSelector(
		(state) => rankedEvaluations(state.evaluation),
		(left, right) =>
			left.length === right.length &&
			left.every((row, index) => row === right[index]),
	);
}

export function useSymmPulseLog(): EnginePulseEvent[] {
	return useSymmSelector((state) => state.pulseLog, arrayEqual);
}

export function useMarketWatchSymbol(fallback = "BTC/EUR"): string {
	return useSymmSelector(
		(state) =>
			pickMarketWatchSymbol(state.fieldSnapshot, state.scoreboard, fallback),
		Object.is,
	);
}

export function useSymmTrades(): Array<TradeEnterEvent | TradeExitEvent> {
	return useSymmSelector((state) => state.trades, arrayEqual);
}

export type TradePanelRow = {
	key: string;
	kind: "open" | "enter" | "exit";
	symbol: string;
	regime: string;
	reason: string;
	pnl_eur?: number;
};

export function selectTradePanelRows(state: SymmUIState): TradePanelRow[] {
	const open = state.status?.positions ?? [];
	const openSymbols = new Set(open.map((p) => p.symbol));
	const rows: TradePanelRow[] = open.map((p) => ({
		key: `open:${p.symbol}`,
		kind: "open",
		symbol: p.symbol,
		regime: p.regime,
		reason: "open",
	}));

	for (const trade of state.trades) {
		if (trade.event === "trade_enter" && openSymbols.has(trade.symbol)) {
			continue;
		}
		rows.push({
			key: `${trade.event}:${trade.ts}:${trade.symbol}`,
			kind: trade.event === "trade_enter" ? "enter" : "exit",
			symbol: trade.symbol,
			regime: trade.regime,
			reason: trade.reason,
			pnl_eur: trade.event === "trade_exit" ? trade.pnl_eur : undefined,
		});
	}
	return rows;
}

export function useSymmTradePanelRows(): TradePanelRow[] {
	return useSymmSelector(selectTradePanelRows, (a, b) => {
		if (a.length !== b.length) return false;
		return a.every((row, index) => {
			const other = b[index];
			return (
				row.key === other.key &&
				row.kind === other.kind &&
				row.symbol === other.symbol &&
				row.regime === other.regime &&
				row.reason === other.reason &&
				row.pnl_eur === other.pnl_eur
			);
		});
	});
}

export type { StatusPosition };

/** Full dashboard state — causes rerender on any telemetry slice change. */
export function useSymmUI(url?: string): SymmUIState {
	useSymmFeed(url);
	return useSyncExternalStore(
		(onStoreChange) =>
			subscribeUISelector(
				(state) => state,
				(a, b) => a === b,
				onStoreChange,
			),
		getUIState,
		getUIState,
	);
}

export type { SymmUIState };
