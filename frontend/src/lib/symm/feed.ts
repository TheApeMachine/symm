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
	StatusEvent,
	SymmEvent,
	TradeEnterEvent,
	TradeExitEvent,
	WatchCommand,
} from "#/lib/symm/events";
import { defaultWsUrl, pickMarketWatchSymbol } from "#/lib/symm/events";
import { positionSymbolsFromStatus } from "#/lib/symm/positions";
import { applyFluidDisplay, fieldStore } from "#/lib/symm/stores/field-store";
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
import {
	applyCandleBar,
	applyChartPosition,
	applyChartSeed,
	chartStore,
	symbolChartState,
} from "#/lib/symm/stores/chart-store";
import { tradeChartBridge } from "#/lib/symm/chart-bridge";
import { WsStream } from "#/lib/symm/ws-stream";

const MAX_CANDLE_HISTORY = 360;

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

const FIELD_EVENTS = new Set<SymmEvent["event"]>([
	"hello",
	"field_snapshot",
	"fluid_display",
]);

const CHART_EVENTS = new Set<SymmEvent["event"]>([
	"hello",
	"price_tick",
	"candle_bar",
	"chart_seed",
	"stop_ratchet",
	"trade_enter",
	"trade_exit",
]);

let chartStream: WsStream | null = null;
let fieldWsStream: WsStream | null = null;
let wsUrl = defaultWsUrl;
let started = false;
let marketWatchSticky = "";

const streams: WsStream[] = [];

const trimCandles = (candles: CandleBarEvent[]) => {
	if (candles.length <= MAX_CANDLE_HISTORY) {
		return candles;
	}

	return candles.slice(candles.length - MAX_CANDLE_HISTORY);
};

const hasChartCandle = (symbol: string) =>
	(chartStore.state.symbols[symbol]?.candles.length ?? 0) > 0;

const chartSubscribeSymbols = () => {
	const watch = pickMarketWatchSymbol(
		statusStore.state.scoreboard,
		fieldStore.state.fieldSnapshot,
		"BTC/EUR",
		marketWatchSticky,
		hasChartCandle,
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

const syncStatusPositions = (status: StatusEvent) => {
	const openSymbols = new Set(
		(status.positions ?? []).map((position) => position.symbol),
	);

	for (const position of status.positions ?? []) {
		applyChartPosition(position.symbol, position);
		tradeChartBridge(position.symbol)?.setPosition(position);
	}

	for (const symbol of Object.keys(chartStore.state.symbols)) {
		if (openSymbols.has(symbol)) {
			continue;
		}

		applyChartPosition(symbol, undefined);
		tradeChartBridge(symbol)?.clearPosition();
	}
};

const applyFieldSnapshot = (snapshot: FieldSnapshotEvent) => {
	fieldStore.setState(() => ({ fieldSnapshot: snapshot }));
	chartSubscribeSymbols();
};

const applyStatusEvent = (status: StatusEvent) => {
	applyStatus(status);
	syncStatusPositions(status);
	chartSubscribeSymbols();
};

const applyScoreboardEvent = (scoreboard: ScoreboardEvent) => {
	applyScoreboard(scoreboard);
	chartSubscribeSymbols();
};

const applyEnginePulseEvent = (pulse: EnginePulseEvent) => {
	applyEnginePulse(pulse);
};

const applyChartEvent = (event: SymmEvent) => {
	switch (event.event) {
		case "candle_bar": {
			const bar = event as CandleBarEvent;
			applyCandleBar(bar);
			tradeChartBridge(String(bar.symbol))?.appendCandle(bar);
			return;
		}
		case "chart_seed": {
			const seed = event as ChartSeedEvent;
			applyChartSeed(seed);
			const symbol = String(seed.symbol);
			const candles = trimCandles(
				symbolChartState(chartStore.state, symbol).candles,
			);
			tradeChartBridge(symbol)?.seedCandles(candles);
			return;
		}
		case "stop_ratchet":
			tradeChartBridge(String(event.symbol))?.ratchetStop(event.new_stop);
			return;
		case "trade_enter":
		case "trade_exit":
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
				applyEnginePulseEvent(event as EnginePulseEvent);
				return;
			}

			if (event.event === "decision_trace") {
				applyDecisionTrace(event as DecisionTraceEvent);
			}
		},
	});

	const fieldStream = new WsStream({
		url: wsUrl,
		stream: "field",
		accepts: FIELD_EVENTS,
		onEvent: (event) => {
			if (event.event === "field_snapshot") {
				applyFieldSnapshot(event as FieldSnapshotEvent);
				return;
			}

			if (event.event === "fluid_display") {
				applyFluidDisplay(event as FluidDisplayEvent);
			}
		},
		onOpen: () => {
			fieldStream.send({ op: "get_fluid_display" });
		},
	});

	fieldWsStream = fieldStream;

	chartStream = new WsStream({
		url: wsUrl,
		stream: "chart",
		accepts: CHART_EVENTS,
		onEvent: applyChartEvent,
		onOpen: chartSubscribeSymbols,
	});

	return [statusStream, engineStream, fieldStream, chartStream];
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
	fieldWsStream = null;
};

export const setFluidDisplay = (patch: FluidDisplayPatch) => {
	const command: WatchCommand = {
		op: "set_fluid_display",
		...patch,
	};

	fieldWsStream?.send(command);
};

export const subscribeTradeChart = (symbol: string): void => {
	const state = symbolChartState(chartStore.state, symbol);
	const bridge = tradeChartBridge(symbol);

	if (state.candles.length > 0) {
		bridge?.seedCandles(state.candles);
	}

	if (state.position) {
		bridge?.setPosition(state.position);
	}

	chartStream?.send({ op: "subscribe", symbols: [symbol] });
};

export const unsubscribeTradeChart = (symbol: string): void => {
	chartStream?.send({ op: "unsubscribe", symbols: [symbol] });
};

export const replayEnginePulseHistory = (
	appendPulse: (pulse: EnginePulseEvent) => void,
): void => {
	const history = [...engineStore.state.pulseLog].reverse();

	for (const pulse of history) {
		appendPulse(pulse);
	}
};

export const replayFieldSnapshot = (
	updateGrid: (snapshot: FieldSnapshotEvent) => void,
): void => {
	const snapshot = fieldStore.state.fieldSnapshot;

	if (snapshot) {
		updateGrid(snapshot);
	}
};

/** @deprecated price ticks are retained for diagnostics only */
export type { PriceTickEvent };
