import type {
	CandleBarEvent,
	ChartSeedEvent,
	DecisionTraceEvent,
	EnginePulseEvent,
	FieldSnapshotEvent,
	FluidDisplayEvent,
	FluidDisplayPatch,
	PriceTickEvent,
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
import { applyFluidDisplay, fieldStore } from "#/lib/symm/stores/field-store";
import {
	applyDecisionTrace,
	applyEnginePulse,
	applySignalScore,
	engineStore,
} from "#/lib/symm/stores/engine-store";
import {
	appendTrade,
	applyScoreboard,
	applyStatus,
	statusStore,
} from "#/lib/symm/stores/status-store";
import { WsStream } from "#/lib/symm/ws-stream";

const MAX_TICK_HISTORY = 360;

type ChartListener = (event: SymmEvent) => void;

const chartListeners = new Map<string, ChartListener>();
const lastTickBySymbol = new Map<string, PriceTickEvent>();
const tickHistoryBySymbol = new Map<string, PriceTickEvent[]>();
const lastCandleBySymbol = new Map<string, CandleBarEvent>();
const candleHistoryBySymbol = new Map<string, CandleBarEvent[]>();
const lastSeedBySymbol = new Map<string, SymmEvent>();
const fieldSnapshotListeners = new Set<
	(snapshot: FieldSnapshotEvent) => void
>();
const enginePulseListeners = new Set<(pulse: EnginePulseEvent) => void>();

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

const appendCandleHistory = (bar: CandleBarEvent) => {
	const symbol = String(bar.symbol);
	const history = candleHistoryBySymbol.get(symbol) ?? [];
	const lastBar = history[history.length - 1];

	if (lastBar?.sec === bar.sec) {
		history[history.length - 1] = bar;
	} else {
		history.push(bar);
	}

	if (history.length > MAX_TICK_HISTORY) {
		history.splice(0, history.length - MAX_TICK_HISTORY);
	}

	candleHistoryBySymbol.set(symbol, history);
};

const hasChartTick = (symbol: string) => lastTickBySymbol.has(symbol);

const chartSubscribeSymbols = () => {
	const watch = pickMarketWatchSymbol(
		statusStore.state.scoreboard,
		fieldStore.state.fieldSnapshot,
		"BTC/EUR",
		marketWatchSticky,
		hasChartTick,
	);
	marketWatchSticky = watch;

	const symbols = new Set<string>([watch]);
	for (const symbol of positionSymbolsFromStatus(statusStore.state.status)) {
		symbols.add(symbol);
	}

	if (symbols.size === 0) {
		return;
	}

	feedStream?.send({ op: "subscribe", symbols: [...symbols] });
};

const notifyFieldSnapshotListeners = (snapshot: FieldSnapshotEvent) => {
	for (const listener of fieldSnapshotListeners) {
		listener(snapshot);
	}
};

const notifyEnginePulseListeners = (pulse: EnginePulseEvent) => {
	for (const listener of enginePulseListeners) {
		listener(pulse);
	}
};

const applyFieldSnapshot = (snapshot: FieldSnapshotEvent) => {
	fieldStore.setState(() => ({ fieldSnapshot: snapshot }));
	notifyFieldSnapshotListeners(snapshot);
	chartSubscribeSymbols();
};

const applyStatusEvent = (status: StatusEvent) => {
	applyStatus(status);
	chartSubscribeSymbols();

	for (const listener of chartListeners.values()) {
		listener(status);
	}
};

const applyScoreboardEvent = (scoreboard: ScoreboardEvent) => {
	applyScoreboard(scoreboard);
	chartSubscribeSymbols();
};

const replayChartState = (symbol: string, handler: ChartListener) => {
	for (const event of buildChartReplayEvents(
		symbol,
		lastSeedBySymbol.get(symbol),
		candleHistoryBySymbol.get(symbol) ?? [],
		statusStore.state.status,
	)) {
		handler(event);
	}
};

const applyChartEvent = (event: SymmEvent) => {
	switch (event.event) {
		case "price_tick": {
			const tick = event as PriceTickEvent;
			lastTickBySymbol.set(String(tick.symbol), tick);
			appendTickHistory(tick);
			dispatchChart(String(tick.symbol), tick);
			return;
		}
		case "candle_bar": {
			const bar = event as CandleBarEvent;
			lastCandleBySymbol.set(String(bar.symbol), bar);
			appendCandleHistory(bar);
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
			dispatchChart(String(event.symbol), event);
			return;
		case "trade_enter":
		case "trade_exit":
			dispatchChart(String(event.symbol), event);
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
			applyStatusEvent(event as StatusEvent);
			return;
		case "scoreboard":
			applyScoreboardEvent(event as ScoreboardEvent);
			return;
		case "trade_enter":
		case "trade_exit":
			appendTrade(event as TradeEnterEvent | TradeExitEvent);
			chartSubscribeSymbols();
			applyChartEvent(event);
			return;
		case "signal_score":
			applySignalScore(event as SignalScoreEvent);
			return;
		case "engine_pulse": {
			const pulse = event as EnginePulseEvent;
			applyEnginePulse(pulse);
			notifyEnginePulseListeners(pulse);
			return;
		}
		case "decision_trace":
			applyDecisionTrace(event as DecisionTraceEvent);
			return;
		case "field_snapshot":
			applyFieldSnapshot(event as FieldSnapshotEvent);
			return;
		case "fluid_display":
			applyFluidDisplay(event as FluidDisplayEvent);
			return;
		case "price_tick":
		case "candle_bar":
		case "chart_seed":
		case "stop_ratchet":
			applyChartEvent(event);
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
};

export const setFluidDisplay = (patch: FluidDisplayPatch) => {
	const command: WatchCommand = {
		op: "set_fluid_display",
		...patch,
	};

	feedStream?.send(command);
};

export const registerChart = (symbol: string, handler: ChartListener) => {
	chartListeners.set(symbol, handler);
	replayChartState(symbol, handler);
	feedStream?.send({ op: "subscribe", symbols: [symbol] });
};

export const unregisterChart = (symbol: string) => {
	chartListeners.delete(symbol);
	feedStream?.send({ op: "unsubscribe", symbols: [symbol] });
};

export const registerFieldSnapshotListener = (
	handler: (snapshot: FieldSnapshotEvent) => void,
) => {
	fieldSnapshotListeners.add(handler);

	const snapshot = fieldStore.state.fieldSnapshot;
	if (snapshot) {
		handler(snapshot);
	}
};

export const unregisterFieldSnapshotListener = (
	handler: (snapshot: FieldSnapshotEvent) => void,
) => {
	fieldSnapshotListeners.delete(handler);
};

export const registerEnginePulseListener = (
	handler: (pulse: EnginePulseEvent) => void,
) => {
	enginePulseListeners.add(handler);

	const pulse = engineStore.state.enginePulse;
	if (pulse) {
		handler(pulse);
	}
};

export const unregisterEnginePulseListener = (
	handler: (pulse: EnginePulseEvent) => void,
) => {
	enginePulseListeners.delete(handler);
};
