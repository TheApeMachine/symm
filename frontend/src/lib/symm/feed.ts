import type {
	ChartSeedEvent,
	DecisionTraceEvent,
	EnginePulseEvent,
	FieldAggregateEvent,
	FieldGridEvent,
	FieldRowEvent,
	FieldSnapshotEvent,
	FluidDisplayEvent,
	FluidDisplayPatch,
	PriceTickEvent,
	QuoteProgressEvent,
	ScoreboardEvent,
	SignalScoreEvent,
	StatusEvent,
	SymmEvent,
	TradeEnterEvent,
	TradeExitEvent,
	WatchCommand,
} from "#/lib/symm/events";
import { defaultWsUrl, pickMarketWatchSymbol } from "#/lib/symm/events";
import { buildChartReplayEvents } from "#/lib/symm/chart-replay";
import { positionSymbolsFromStatus } from "#/lib/symm/positions";
import {
	appendTrade,
	applyDecisionTrace,
	applyEnginePulse,
	applyFieldAggregate,
	applyFieldGrid,
	applyFieldRow,
	applyFieldSnapshot,
	applyFluidDisplay,
	applyFluidSampled,
	applyQuoteProgress,
	applyScoreboard,
	applySignalScore,
	applyStatus,
	buildFieldSnapshot,
	dashboardStore,
	setFeedConnected,
} from "#/lib/symm/dashboard-store";
import { WsStream } from "#/lib/symm/ws-stream";

const MAX_TICK_HISTORY = 360;

type ChartListener = (event: SymmEvent) => void;

const chartListeners = new Map<string, ChartListener>();
const tickHistoryBySymbol = new Map<string, PriceTickEvent[]>();
const lastSeedBySymbol = new Map<string, ChartSeedEvent>();

let feedStream: WsStream | null = null;
let wsUrl = defaultWsUrl;
let started = false;
let marketWatchSticky = "";

const dispatchChart = (symbol: string, event: SymmEvent) => {
	chartListeners.get(symbol)?.(event);
};

const appendTickHistory = (tick: PriceTickEvent) => {
	const symbol = String(tick.symbol);
	const history = tickHistoryBySymbol.get(symbol) ?? [];
	history.push(tick);

	if (history.length > MAX_TICK_HISTORY) {
		history.splice(0, history.length - MAX_TICK_HISTORY);
	}

	tickHistoryBySymbol.set(symbol, history);
};

const hasChartTick = (symbol: string) => {
	const history = tickHistoryBySymbol.get(symbol);
	return history !== undefined && history.length > 0;
};

const chartSubscribeSymbols = () => {
	const watch = pickMarketWatchSymbol(
		dashboardStore.state.scoreboard,
		buildFieldSnapshot(dashboardStore.state),
		"BTC/EUR",
		marketWatchSticky,
		hasChartTick,
	);
	marketWatchSticky = watch;

	const symbols = new Set<string>([watch]);
	for (const symbol of positionSymbolsFromStatus(dashboardStore.state.status)) {
		symbols.add(symbol);
	}

	if (symbols.size === 0) {
		return;
	}

	feedStream?.send({ op: "subscribe", symbols: [...symbols] });
};

const replayChartState = (symbol: string, handler: ChartListener) => {
	for (const event of buildChartReplayEvents(
		symbol,
		lastSeedBySymbol.get(symbol),
		tickHistoryBySymbol.get(symbol) ?? [],
		dashboardStore.state.status,
	)) {
		handler(event);
	}
};

const routeChartEvent = (event: SymmEvent) => {
	switch (event.event) {
		case "price_tick": {
			const tick = event as PriceTickEvent;
			appendTickHistory(tick);
			dispatchChart(String(tick.symbol), tick);
			return;
		}
		case "chart_seed": {
			const seed = event as ChartSeedEvent;
			lastSeedBySymbol.set(String(seed.symbol), seed);
			dispatchChart(String(seed.symbol), seed);
			return;
		}
		case "stop_ratchet":
		case "trade_enter":
		case "trade_exit":
		case "status":
			dispatchChart(String(event.symbol ?? ""), event);
			return;
		default:
			return;
	}
};

const handleFeedEvent = (event: SymmEvent) => {
	switch (event.event) {
		case "hello":
			return;
		case "status":
			applyStatus(event as StatusEvent);
			chartSubscribeSymbols();
			for (const listener of chartListeners.values()) {
				listener(event);
			}
			return;
		case "scoreboard":
			applyScoreboard(event as ScoreboardEvent);
			chartSubscribeSymbols();
			return;
		case "trade_enter":
		case "trade_exit":
			appendTrade(event as TradeEnterEvent | TradeExitEvent);
			chartSubscribeSymbols();
			routeChartEvent(event);
			return;
		case "signal_score":
			applySignalScore(event as SignalScoreEvent);
			return;
		case "engine_pulse":
			applyEnginePulse(event as EnginePulseEvent);
			return;
		case "decision_trace":
			applyDecisionTrace(event as DecisionTraceEvent);
			return;
		case "quote_progress": {
			const progress = event as QuoteProgressEvent;
			applyQuoteProgress(progress.ready, progress.total);
			return;
		}
		case "field_snapshot":
			applyFieldSnapshot(event as FieldSnapshotEvent);
			chartSubscribeSymbols();
			return;
		case "field_row": {
			const rowEvent = event as FieldRowEvent;
			applyFieldRow(rowEvent.row);
			applyFluidSampled(Object.keys(dashboardStore.state.rows).length);
			return;
		}
		case "field_aggregate":
			applyFieldAggregate(
				(event as FieldAggregateEvent).symbol_count,
				(event as FieldAggregateEvent).field,
			);
			applyFluidSampled((event as FieldAggregateEvent).symbol_count);
			return;
		case "field_grid":
			applyFieldGrid((event as FieldGridEvent).grid);
			return;
		case "fluid_display":
			applyFluidDisplay(event as FluidDisplayEvent);
			return;
		case "price_tick":
		case "chart_seed":
		case "stop_ratchet":
			routeChartEvent(event);
			return;
		default:
			return;
	}
};

const openFeedStream = () => {
	feedStream = new WsStream({
		url: wsUrl,
		onEvent: handleFeedEvent,
		onOpen: () => {
			feedStream?.send({ op: "get_fluid_display" });
			chartSubscribeSymbols();
		},
	});
	feedStream.start();
};

export const startSymmFeed = (url: string = defaultWsUrl) => {
	wsUrl = url;
	if (started) {
		return;
	}

	started = true;
	openFeedStream();
};

export const stopSymmFeed = () => {
	if (!started) {
		return;
	}

	started = false;
	feedStream?.stop();
	feedStream = null;
	setFeedConnected(false);
};

export const setFluidDisplay = (patch: FluidDisplayPatch) => {
	const command: WatchCommand = {
		op: "set_fluid_display",
		...patch,
	};

	feedStream?.send(command);
};

export const onChart = (
	symbol: string,
	handler: ChartListener,
): (() => void) => {
	chartListeners.set(symbol, handler);
	replayChartState(symbol, handler);
	feedStream?.send({ op: "subscribe", symbols: [symbol] });

	return () => {
		chartListeners.delete(symbol);
		feedStream?.send({ op: "unsubscribe", symbols: [symbol] });
	};
};
