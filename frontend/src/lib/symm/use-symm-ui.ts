import { useEffect, useMemo } from "react";
import { useSelector } from "@tanstack/react-store";

import type {
	DecisionTraceEvent,
	EnginePulseEvent,
	EvaluationRow,
	FieldSnapshotEvent,
	FluidDisplayEvent,
	ScoreboardEvent,
	StatusEvent,
	StatusPosition,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";
import { pickMarketWatchSymbol } from "#/lib/symm/events";
import {
	buildFieldSnapshot,
	dashboardStore,
} from "#/lib/symm/dashboard-store";
import type { SignalConfidenceSnapshot } from "#/lib/symm/signal-confidence";
import { positionSymbolsFromStatus } from "#/lib/symm/positions";
import {
	selectTradePanelRows,
	type TradePanelRow,
} from "#/lib/symm/trade-panel";
import { startSymmFeed, stopSymmFeed } from "#/lib/symm/feed";

export function useSymmFeed(url?: string): void {
	useEffect(() => {
		startSymmFeed(url);

		return () => {
			stopSymmFeed();
		};
	}, [url]);
}

export function useSymmConnected(): boolean {
	return useSelector(dashboardStore, (state) => state.connected);
}

export function useSymmStatus(): StatusEvent | undefined {
	return useSelector(dashboardStore, (state) => state.status);
}

export function useSymmPositionSymbols(): string[] {
	return useSelector(dashboardStore, (state) =>
		positionSymbolsFromStatus(state.status),
	);
}

export function useSymmScoreboard(): ScoreboardEvent | undefined {
	return useSelector(dashboardStore, (state) => state.scoreboard);
}

export function useSymmDecisionTrace(): DecisionTraceEvent | undefined {
	return useSelector(dashboardStore, (state) => state.decisionTrace);
}

export function useSymmScanProgress(): {
	quotesReady: number;
	symbolsTotal?: number;
	fluidSampled: number;
} {
	return useSelector(dashboardStore, (state) => ({
		quotesReady: state.quotesReady,
		symbolsTotal: state.symbolsTotal,
		fluidSampled: Math.max(
			state.fluidSampled,
			state.symbolCount || Object.keys(state.rows).length,
		),
	}));
}

export function useSymmFieldSnapshot(): FieldSnapshotEvent | undefined {
	return useSelector(dashboardStore, (state) => buildFieldSnapshot(state));
}

export function useSymmFluidDisplay(): FluidDisplayEvent | undefined {
	return useSelector(dashboardStore, (state) => state.fluidDisplay);
}

export function useSymmEnginePulse(): EnginePulseEvent | undefined {
	return useSelector(dashboardStore, (state) => state.enginePulse);
}

export function useSymmEvaluations(): EvaluationRow[] {
	return useSelector(
		dashboardStore,
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
	return useSelector(dashboardStore, (state) => state.signalConfidences);
}

export function useSymmPulseLog(): EnginePulseEvent[] {
	return useSelector(dashboardStore, (state) => state.pulseLog);
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
	return useSelector(dashboardStore, (state) => state.trades);
}

export function useSymmTradePanelRows(): TradePanelRow[] {
	const status = useSelector(dashboardStore, (state) => state.status);
	const trades = useSelector(dashboardStore, (state) => state.trades);

	return useMemo(
		() => selectTradePanelRows({ status, trades }),
		[status, trades],
	);
}

export type { StatusPosition, TradePanelRow };
