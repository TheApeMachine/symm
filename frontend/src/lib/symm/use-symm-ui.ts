import { useEffect, useMemo } from "react";
import { useSelector } from "@tanstack/react-store";

import type {
	DecisionTraceEvent,
	EnginePulseEvent,
	EvaluationRow,
	FieldSnapshotEvent,
	ScoreboardEvent,
	StatusEvent,
	StatusPosition,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";
import { pickMarketWatchSymbol } from "#/lib/symm/events";
import type { SignalConfidenceSnapshot } from "#/lib/symm/signal-confidence";
import { positionSymbolsFromStatus } from "#/lib/symm/positions";
import {
	selectTradePanelRows,
	type TradePanelRow,
} from "#/lib/symm/trade-panel";
import { connectionStore } from "#/lib/symm/stores/connection-store";
import { engineStore } from "#/lib/symm/stores/engine-store";
import { fieldStore } from "#/lib/symm/stores/field-store";
import { statusStore } from "#/lib/symm/stores/status-store";
import { startSymmFeed } from "#/lib/symm/feed";

export function useSymmFeed(url?: string): void {
	useEffect(() => {
		startSymmFeed(url);
	}, [url]);
}

export function useSymmConnected(): boolean {
	return useSelector(connectionStore, (state) =>
		Object.values(state).every(Boolean),
	);
}

export function useSymmStatus(): StatusEvent | undefined {
	return useSelector(statusStore, (state) => state.status);
}

export function useSymmPositionSymbols(): string[] {
	return useSelector(statusStore, (state) =>
		positionSymbolsFromStatus(state.status),
	);
}

export function useSymmScoreboard(): ScoreboardEvent | undefined {
	return useSelector(statusStore, (state) => state.scoreboard);
}

export function useSymmDecisionTrace(): DecisionTraceEvent | undefined {
	return useSelector(engineStore, (state) => state.decisionTrace);
}

export function useSymmFieldSnapshot(): FieldSnapshotEvent | undefined {
	return useSelector(fieldStore, (state) => state.fieldSnapshot);
}

export function useSymmEnginePulse(): EnginePulseEvent | undefined {
	return useSelector(engineStore, (state) => state.enginePulse);
}

export function useSymmEvaluations(): EvaluationRow[] {
	return useSelector(
		engineStore,
		(state) => state.decisionTrace?.evaluations ?? [],
	);
}

export function useSymmEntryLine(): {
	line: number;
	median: number;
	mad: number;
} {
	const trace = useSymmDecisionTrace();
	const scoreboard = useSymmScoreboard();

	return useMemo(
		() => ({
			line: trace?.line ?? scoreboard?.line ?? 0,
			median: trace?.median ?? scoreboard?.median ?? 0,
			mad: trace?.mad ?? scoreboard?.mad ?? 0,
		}),
		[trace, scoreboard],
	);
}

export function useSymmSignalConfidences(): SignalConfidenceSnapshot {
	return useSelector(engineStore, (state) => state.signalConfidences);
}

export function useSymmPulseLog(): EnginePulseEvent[] {
	return useSelector(engineStore, (state) => state.pulseLog);
}

export function useMarketWatchSymbol(fallback = "BTC/EUR"): string {
	const scoreboard = useSymmScoreboard();
	const fieldSnapshot = useSymmFieldSnapshot();

	return useMemo(
		() => pickMarketWatchSymbol(scoreboard, fieldSnapshot, fallback),
		[scoreboard, fieldSnapshot, fallback],
	);
}

export function useSymmTrades(): Array<TradeEnterEvent | TradeExitEvent> {
	return useSelector(statusStore, (state) => state.trades);
}

export function useSymmTradePanelRows(): TradePanelRow[] {
	const status = useSelector(statusStore, (state) => state.status);
	const trades = useSelector(statusStore, (state) => state.trades);

	return useMemo(
		() => selectTradePanelRows({ status, trades }),
		[status, trades],
	);
}

export type { StatusPosition, TradePanelRow };
