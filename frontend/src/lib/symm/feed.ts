import type {
	CandleBarEvent,
	ChartSeedEvent,
	DecisionTraceEvent,
	EnginePulseEvent,
	FieldAggregateEvent,
	FieldGridEvent,
	FieldRowEvent,
	FieldSnapshotEvent,
	FluidDisplayEvent,
	FluidDisplayPatch,
	QuoteProgressEvent,
	ScoreboardEvent,
	SignalScoreEvent,
	StatusEvent,
	SymmEvent,
	TradeEnterEvent,
	TradeExitEvent,
	WatchCommand,
} from "#/lib/symm/events";
import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
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
import {
	confidenceToGaugePercent,
	isSignalSource,
	type SignalSource,
} from "#/lib/symm/signal-confidence";
import { WsStream } from "#/lib/symm/ws-stream";

const MAX_CANDLE_HISTORY = 720;

type FluidSurfaceListener = (snapshot: FieldSnapshotEvent) => void;
type EnginePulseListener = (pulse: EnginePulseEvent) => void;
type SignalGaugeListener = (needlePercent: number, confidence: number) => void;

const fluidSurfaceListeners = new Set<FluidSurfaceListener>();
const enginePulseListeners = new Set<EnginePulseListener>();
const signalGaugeListeners = new Map<SignalSource, Set<SignalGaugeListener>>();

const dispatchFluidSurface = () => {
	const snapshot = buildFieldSnapshot(dashboardStore.state);

	if (!snapshot) {
		return;
	}

	for (const listener of fluidSurfaceListeners) {
		listener(snapshot);
	}
};

const dispatchEnginePulse = (pulse: EnginePulseEvent) => {
	for (const listener of enginePulseListeners) {
		listener(pulse);
	}
};

const dispatchSignalGauge = (event: SignalScoreEvent) => {
	if (!isSignalSource(event.source)) {
		return;
	}

	const listeners = signalGaugeListeners.get(event.source);

	if (!listeners) {
		return;
	}

	const confidence =
		typeof event.confidence === "number" && Number.isFinite(event.confidence)
			? event.confidence
			: 0;
	const needlePercent = confidenceToGaugePercent(confidence);

	for (const listener of listeners) {
		listener(needlePercent, confidence);
	}
};

type ChartListener = (event: SymmEvent) => void;

const chartListeners = new Map<string, Set<ChartListener>>();
const candleHistoryBySymbol = new Map<string, CandleBarEvent[]>();
const lastSeedBySymbol = new Map<string, ChartSeedEvent>();

let feedStream: WsStream | null = null;
let wsUrl = defaultWsUrl;
let started = false;
let marketWatchSticky = "";

const dispatchChart = (symbol: string, event: SymmEvent) => {
	const listeners = chartListeners.get(symbol);

	if (!listeners) {
		return;
	}

	for (const listener of listeners) {
		listener(event);
	}
};

const appendCandleHistory = (bar: CandleBarEvent) => {
	const symbol = String(bar.symbol);
	const history = candleHistoryBySymbol.get(symbol) ?? [];
	const last = history[history.length - 1];

	if (last && last.sec === bar.sec) {
		history[history.length - 1] = bar;
		candleHistoryBySymbol.set(symbol, history);
		return;
	}

	history.push(bar);

	if (history.length > MAX_CANDLE_HISTORY) {
		history.splice(0, history.length - MAX_CANDLE_HISTORY);
	}

	candleHistoryBySymbol.set(symbol, history);
};

const hasChartData = (symbol: string) => {
	const history = candleHistoryBySymbol.get(symbol);
	return history !== undefined && history.length > 0;
};

const chartSubscribeSymbols = () => {
	const watch = pickMarketWatchSymbol(
		dashboardStore.state.scoreboard,
		buildFieldSnapshot(dashboardStore.state),
		"BTC/EUR",
		marketWatchSticky,
		hasChartData,
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
		candleHistoryBySymbol.get(symbol) ?? [],
		dashboardStore.state.status,
	)) {
		handler(event);
	}
};

const routeChartEvent = (event: SymmEvent) => {
	switch (event.event) {
		case "candle_bar": {
			const bar = event as CandleBarEvent;
			appendCandleHistory(bar);
			OhlcDataProvider.ingest(bar);
			dispatchChart(String(bar.symbol), bar);
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

			for (const listeners of chartListeners.values()) {
				for (const listener of listeners) {
					listener(event);
				}
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
		case "signal_score": {
			const score = event as SignalScoreEvent;
			applySignalScore(score);
			dispatchSignalGauge(score);
			return;
		}
		case "engine_pulse": {
			const pulse = event as EnginePulseEvent;
			applyEnginePulse(pulse);
			dispatchEnginePulse(pulse);
			return;
		}
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
			dispatchFluidSurface();
			return;
		case "field_row": {
			const rowEvent = event as FieldRowEvent;
			applyFieldRow(rowEvent.row);
			applyFluidSampled(Object.keys(dashboardStore.state.rows).length);
			dispatchFluidSurface();
			return;
		}
		case "field_aggregate":
			applyFieldAggregate(
				(event as FieldAggregateEvent).symbol_count,
				(event as FieldAggregateEvent).field,
			);
			applyFluidSampled((event as FieldAggregateEvent).symbol_count);
			dispatchFluidSurface();
			return;
		case "field_grid":
			applyFieldGrid((event as FieldGridEvent).grid);
			dispatchFluidSurface();
			return;
		case "fluid_display":
			applyFluidDisplay(event as FluidDisplayEvent);
			return;
		case "candle_bar":
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
	const listeners = chartListeners.get(symbol) ?? new Set<ChartListener>();
	listeners.add(handler);
	chartListeners.set(symbol, listeners);
	replayChartState(symbol, handler);
	feedStream?.send({ op: "subscribe", symbols: [symbol] });

	return () => {
		const current = chartListeners.get(symbol);

		if (!current) {
			return;
		}

		current.delete(handler);

		if (current.size === 0) {
			chartListeners.delete(symbol);
		}

		feedStream?.send({ op: "unsubscribe", symbols: [symbol] });
	};
};

export const registerFluidSurface = (
	handler: FluidSurfaceListener,
): (() => void) => {
	fluidSurfaceListeners.add(handler);
	const snapshot = buildFieldSnapshot(dashboardStore.state);

	if (snapshot) {
		handler(snapshot);
	}

	return () => {
		fluidSurfaceListeners.delete(handler);
	};
};

export const registerEnginePulseChart = (
	appendPulse: EnginePulseListener,
): (() => void) => {
	enginePulseListeners.add(appendPulse);
	const pulse = dashboardStore.state.enginePulse;

	if (pulse) {
		appendPulse(pulse);
	}

	return () => {
		enginePulseListeners.delete(appendPulse);
	};
};

export const registerSignalGauge = (
	source: SignalSource,
	handler: SignalGaugeListener,
): (() => void) => {
	const listeners = signalGaugeListeners.get(source) ?? new Set();
	listeners.add(handler);
	signalGaugeListeners.set(source, listeners);

	const confidence = dashboardStore.state.signalConfidences[source];

	if (confidence > 0) {
		handler(confidenceToGaugePercent(confidence), confidence);
	}

	return () => {
		const current = signalGaugeListeners.get(source);

		if (!current) {
			return;
		}

		current.delete(handler);

		if (current.size === 0) {
			signalGaugeListeners.delete(source);
		}
	};
};
