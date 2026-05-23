import type {
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
import { defaultWsUrl } from "#/lib/symm/events";
import {
	emptyEvaluationState,
	mergeDecisionTrace,
	mergeEnginePulse,
	type EvaluationState,
} from "#/lib/symm/evaluation-store";

const MAX_TRADES = 40;
const MAX_PULSE_LOG = 32;

export type SymmUIState = {
	connected: boolean;
	status?: StatusEvent;
	scoreboard?: ScoreboardEvent;
	decisionTrace?: DecisionTraceEvent;
	fieldSnapshot?: FieldSnapshotEvent;
	enginePulse?: EnginePulseEvent;
	evaluation: EvaluationState;
	pulseLog: EnginePulseEvent[];
	trades: Array<TradeEnterEvent | TradeExitEvent>;
};

type ChartListener = (event: SymmEvent) => void;

type UISubscription = {
	listener: () => void;
	selector: (state: SymmUIState) => unknown;
	equals: (a: unknown, b: unknown) => boolean;
	snapshot: unknown;
};

let ui: SymmUIState = {
	connected: false,
	evaluation: emptyEvaluationState(),
	pulseLog: [],
	trades: [],
};

const uiSubscriptions = new Set<UISubscription>();
const chartListeners = new Map<string, ChartListener>();
const lastTickBySymbol = new Map<string, PriceTickEvent>();
const lastSeedBySymbol = new Map<string, SymmEvent>();

let fluidSurfaceHandler: ((snapshot: FieldSnapshotEvent) => void) | null = null;
let fieldStreamHandler: ((snapshot: FieldSnapshotEvent) => void) | null = null;
let ws: WebSocket | null = null;
let wsUrl = defaultWsUrl;
let started = false;

const EMPTY_SYMBOLS: string[] = [];

function notifyUI() {
	for (const sub of uiSubscriptions) {
		const next = sub.selector(ui);
		if (!sub.equals(sub.snapshot, next)) {
			sub.snapshot = next;
			sub.listener();
		}
	}
}

function dispatchChart(symbol: string, event: SymmEvent) {
	chartListeners.get(symbol)?.(event);
}

function dispatchAllCharts(event: SymmEvent) {
	for (const listener of chartListeners.values()) {
		listener(event);
	}
}

function subscribeOpenSymbols(positions: StatusEvent["positions"]) {
	if (!ws || ws.readyState !== WebSocket.OPEN || !positions?.length) {
		return;
	}

	const symbols = positions.map((position) => position.symbol).filter(Boolean);
	if (symbols.length === 0) {
		return;
	}

	ws.send(JSON.stringify({ op: "subscribe", symbols }));
}

function replayChartState(symbol: string, handler: ChartListener) {
	const seed = lastSeedBySymbol.get(symbol);
	if (seed) {
		handler(seed);
	}

	if (ui.status) {
		handler(ui.status);
	}

	const tick = lastTickBySymbol.get(symbol);
	if (tick) {
		handler(tick);
	}
}

function tradeKey(trade: TradeEnterEvent | TradeExitEvent) {
	return `${trade.event}:${trade.ts}:${trade.symbol}`;
}

function appendTrade(trade: TradeEnterEvent | TradeExitEvent) {
	const key = tradeKey(trade);
	if (ui.trades.some((row) => tradeKey(row) === key)) {
		return;
	}

	ui = {
		...ui,
		trades: [trade, ...ui.trades].slice(0, MAX_TRADES),
	};
}

function applyEvent(ev: SymmEvent) {
	switch (ev.event) {
		case "price_tick": {
			const tick = ev as PriceTickEvent;
			const prev = lastTickBySymbol.get(String(tick.symbol));
			if (
				prev &&
				prev.last === tick.last &&
				prev.bid === tick.bid &&
				prev.ask === tick.ask
			) {
				return;
			}

			lastTickBySymbol.set(String(tick.symbol), tick);
			dispatchChart(String(tick.symbol), tick);
			return;
		}
		case "chart_seed": {
			const seed = ev;
			lastSeedBySymbol.set(String(seed.symbol), seed);
			dispatchChart(String(seed.symbol), seed);
			return;
		}
		case "stop_ratchet":
			dispatchChart(String(ev.symbol), ev);
			return;
		case "trade_enter":
		case "trade_exit": {
			const trade = ev as TradeEnterEvent | TradeExitEvent;
			appendTrade(trade);
			dispatchChart(trade.symbol, ev);
			notifyUI();
			return;
		}
		case "status": {
			const status = ev as StatusEvent;
			ui = {
				...ui,
				status,
			};
			subscribeOpenSymbols(status.positions);
			dispatchAllCharts(ev);
			notifyUI();
			return;
		}
		case "scoreboard":
			ui = {
				...ui,
				scoreboard: ev as ScoreboardEvent,
				evaluation: {
					...ui.evaluation,
					line: (ev as ScoreboardEvent).line,
					median: (ev as ScoreboardEvent).median,
					mad: (ev as ScoreboardEvent).mad,
				},
			};
			notifyUI();
			return;
		case "decision_trace": {
			const trace = ev as DecisionTraceEvent;
			ui = {
				...ui,
				decisionTrace: trace,
				evaluation: mergeDecisionTrace(ui.evaluation, trace),
			};
			notifyUI();
			return;
		}
		case "field_snapshot": {
			const snapshot = ev as FieldSnapshotEvent;
			ui = {
				...ui,
				fieldSnapshot: snapshot,
			};
			fluidSurfaceHandler?.(snapshot);
			fieldStreamHandler?.(snapshot);
			notifyUI();
			return;
		}
		case "engine_pulse": {
			const pulse = ev as EnginePulseEvent;
			ui = {
				...ui,
				enginePulse: pulse,
				evaluation: mergeEnginePulse(ui.evaluation, pulse),
				pulseLog: [pulse, ...ui.pulseLog].slice(0, MAX_PULSE_LOG),
			};
			notifyUI();
			return;
		}
		default:
			return;
	}
}

function connect() {
	if (ws) {
		ws.close();
		ws = null;
	}

	const socket = new WebSocket(wsUrl);
	ws = socket;

	socket.onopen = () => {
		ui = {
			...ui,
			connected: true,
		};
		notifyUI();
		subscribeOpenSymbols(ui.status?.positions);
	};

	socket.onclose = () => {
		ui = {
			...ui,
			connected: false,
		};
		notifyUI();
		if (started) {
			setTimeout(connect, 2000);
		}
	};

	socket.onerror = () => {
		socket.close();
	};

	socket.onmessage = (message) => {
		try {
			applyEvent(JSON.parse(String(message.data)) as SymmEvent);
		} catch {
			// ignore malformed frames
		}
	};
}

export function startSymmFeed(url: string = defaultWsUrl) {
	wsUrl = url;
	if (started) {
		return;
	}
	started = true;
	connect();
}

export function arrayEqual<T>(left: T[], right: T[]) {
	if (left.length !== right.length) {
		return false;
	}

	return left.every((value, index) => value === right[index]);
}

export function selectPositionSymbols(state: SymmUIState) {
	return (
		state.status?.positions?.map((position) => position.symbol) ?? EMPTY_SYMBOLS
	);
}

export function subscribeUISelector<T>(
	selector: (state: SymmUIState) => T,
	equals: (a: T, b: T) => boolean,
	onStoreChange: () => void,
) {
	const sub: UISubscription = {
		listener: onStoreChange,
		selector: selector as UISubscription["selector"],
		equals: equals as UISubscription["equals"],
		snapshot: selector(ui),
	};

	uiSubscriptions.add(sub);

	return () => {
		uiSubscriptions.delete(sub);
	};
}

export function getUIState() {
	return ui;
}

export function registerChart(symbol: string, handler: ChartListener) {
	chartListeners.set(symbol, handler);
	replayChartState(symbol, handler);

	if (ws?.readyState === WebSocket.OPEN) {
		ws.send(JSON.stringify({ op: "subscribe", symbols: [symbol] }));
	}
}

export function unregisterChart(symbol: string) {
	chartListeners.delete(symbol);
}

export function registerFieldStream(
	handler: (snapshot: FieldSnapshotEvent) => void,
) {
	fieldStreamHandler = handler;
	if (ui.fieldSnapshot) {
		handler(ui.fieldSnapshot);
	}
}

export function unregisterFieldStream() {
	fieldStreamHandler = null;
}

export function registerFluidSurface(
	handler: (snapshot: FieldSnapshotEvent) => void,
) {
	fluidSurfaceHandler = handler;
	if (ui.fieldSnapshot) {
		handler(ui.fieldSnapshot);
	}
}

export function unregisterFluidSurface() {
	fluidSurfaceHandler = null;
}
