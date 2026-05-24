import type {
	ChartSeedEvent,
	DecisionTraceEvent,
	EnginePulseEvent,
	FieldSnapshotEvent,
	PriceTickEvent,
	ScoreboardEvent,
	StatusEvent,
	SymmEvent,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";
import { defaultWsUrl, pickMarketWatchSymbol } from "#/lib/symm/events";
import { buildChartReplayEvents } from "#/lib/symm/chart-replay";
import { positionSymbolsFromStatus } from "#/lib/symm/positions";
import { fieldStore } from "#/lib/symm/stores/field-store";
import {
	applyDecisionTrace,
	applyEnginePulse,
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

const STATUS_EVENTS = new Set<SymmEvent["event"]>([
	"hello",
	"status",
	"trade_enter",
	"trade_exit",
	"scoreboard",
]);

const ENGINE_EVENTS = new Set<SymmEvent["event"]>([
	"hello",
	"engine_pulse",
	"decision_trace",
]);

const FIELD_EVENTS = new Set<SymmEvent["event"]>(["hello", "field_snapshot"]);

const CHART_EVENTS = new Set<SymmEvent["event"]>([
	"hello",
	"price_tick",
	"chart_seed",
	"stop_ratchet",
	"trade_enter",
	"trade_exit",
]);

type ChartListener = (event: SymmEvent) => void;

const chartListeners = new Map<string, ChartListener>();
const lastTickBySymbol = new Map<string, PriceTickEvent>();
const tickHistoryBySymbol = new Map<string, PriceTickEvent[]>();
const lastSeedBySymbol = new Map<string, SymmEvent>();
const fieldSnapshotListeners = new Set<
	(snapshot: FieldSnapshotEvent) => void
>();
const enginePulseListeners = new Set<(pulse: EnginePulseEvent) => void>();

let chartStream: WsStream | null = null;
let wsUrl = defaultWsUrl;
let started = false;
let marketWatchSticky = "";

const streams: WsStream[] = [];

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

	chartStream?.send({ op: "subscribe", symbols: [...symbols] });
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
		tickHistoryBySymbol.get(symbol) ?? [],
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

const createStreams = () => {
	const statusStream = new WsStream({
		url: wsUrl,
		stream: "status",
		accepts: STATUS_EVENTS,
		onEvent: (event) => {
			switch (event.event) {
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
					return;
				default:
					return;
			}
		},
	});

	const engineStream = new WsStream({
		url: wsUrl,
		stream: "engine",
		accepts: ENGINE_EVENTS,
		onEvent: (event) => {
			if (event.event === "engine_pulse") {
				const pulse = event as EnginePulseEvent;
				applyEnginePulse(pulse);
				notifyEnginePulseListeners(pulse);
				return;
			}

			if (event.event === "decision_trace") {
				applyDecisionTrace(event as DecisionTraceEvent);
			}
		},
	});

	const fieldWsStream = new WsStream({
		url: wsUrl,
		stream: "field",
		accepts: FIELD_EVENTS,
		onEvent: (event) => {
			if (event.event === "field_snapshot") {
				applyFieldSnapshot(event as FieldSnapshotEvent);
			}
		},
	});

	chartStream = new WsStream({
		url: wsUrl,
		stream: "chart",
		accepts: CHART_EVENTS,
		onEvent: applyChartEvent,
		onOpen: chartSubscribeSymbols,
	});

	return [statusStream, engineStream, fieldWsStream, chartStream];
};

export const startSymmFeed = (url: string = defaultWsUrl) => {
	wsUrl = url;
	if (started) {
		return;
	}

	started = true;

	for (const stream of createStreams()) {
		streams.push(stream);
		stream.start();
	}
};

export const stopSymmFeed = () => {
	if (!started) {
		return;
	}

	started = false;

	for (const stream of streams) {
		stream.stop();
	}

	streams.length = 0;
	chartStream = null;
};

export const registerChart = (symbol: string, handler: ChartListener) => {
	chartListeners.set(symbol, handler);
	replayChartState(symbol, handler);
	chartStream?.send({ op: "subscribe", symbols: [symbol] });
};

export const unregisterChart = (symbol: string) => {
	chartListeners.delete(symbol);
	chartStream?.send({ op: "unsubscribe", symbols: [symbol] });
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
