import { createStore } from "@tanstack/react-store";

import type {
	DecisionTraceEvent,
	EnginePulseEvent,
	FieldAggregate,
	FieldSnapshotEvent,
	FluidDisplayEvent,
	FluidGridPayload,
	FluidSymbolRow,
	ScoreboardEvent,
	SignalScoreEvent,
	StatusEvent,
	TradeEnterEvent,
	TradeExitEvent,
} from "#/lib/symm/events";
import {
	emptySignalConfidences,
	isSignalSource,
	type SignalConfidenceSnapshot,
} from "#/lib/symm/signal-confidence";

const MAX_PULSE_LOG = 32;
const MAX_TRADES = 40;

export type DashboardState = {
	connected: boolean;
	quotesReady: number;
	symbolsTotal?: number;
	fluidSampled: number;
	status?: StatusEvent;
	scoreboard?: ScoreboardEvent;
	trades: Array<TradeEnterEvent | TradeExitEvent>;
	enginePulse?: EnginePulseEvent;
	decisionTrace?: DecisionTraceEvent;
	pulseLog: EnginePulseEvent[];
	signalConfidences: SignalConfidenceSnapshot;
	rows: Record<string, FluidSymbolRow>;
	aggregate?: FieldAggregate;
	symbolCount: number;
	grid?: FluidGridPayload;
	fluidDisplay?: FluidDisplayEvent;
};

export const dashboardStore = createStore<DashboardState>({
	connected: false,
	quotesReady: 0,
	fluidSampled: 0,
	trades: [],
	pulseLog: [],
	signalConfidences: emptySignalConfidences(),
	rows: {},
	symbolCount: 0,
});

export const setFeedConnected = (connected: boolean): void => {
	dashboardStore.setState((state) => {
		if (state.connected === connected) {
			return state;
		}

		return { ...state, connected };
	});
};

export const applyQuoteProgress = (ready: number, total: number): void => {
	dashboardStore.setState((state) => ({
		...state,
		quotesReady: ready,
		symbolsTotal: total,
	}));
};

export const applyFluidSampled = (sampled: number): void => {
	dashboardStore.setState((state) => ({
		...state,
		fluidSampled: sampled,
	}));
};

const tradeKey = (trade: TradeEnterEvent | TradeExitEvent) =>
	`${trade.event}:${trade.ts}:${trade.symbol}`;

export const pruneClosedTradeEnters = (
	trades: Array<TradeEnterEvent | TradeExitEvent>,
	openSymbols: ReadonlySet<string>,
): Array<TradeEnterEvent | TradeExitEvent> =>
	trades.filter(
		(trade) => trade.event !== "trade_enter" || openSymbols.has(trade.symbol),
	);

export const appendTrade = (trade: TradeEnterEvent | TradeExitEvent): void => {
	dashboardStore.setState((state) => {
		const key = tradeKey(trade);

		if (state.trades.some((row) => tradeKey(row) === key)) {
			return state;
		}

		return {
			...state,
			trades: [trade, ...state.trades].slice(0, MAX_TRADES),
		};
	});
};

export const applyStatus = (status: StatusEvent): void => {
	const openSymbols = new Set(
		status.positions?.map((position) => position.symbol) ?? [],
	);

	dashboardStore.setState((state) => ({
		...state,
		status,
		trades: pruneClosedTradeEnters(state.trades, openSymbols),
	}));
};

export const applyScoreboard = (scoreboard: ScoreboardEvent): void => {
	dashboardStore.setState((state) => ({
		...state,
		scoreboard,
	}));
};

export const applyEnginePulse = (pulse: EnginePulseEvent): void => {
	dashboardStore.setState((state) => ({
		...state,
		enginePulse: pulse,
		pulseLog: [pulse, ...state.pulseLog].slice(0, MAX_PULSE_LOG),
	}));
};

export const applySignalScore = (event: SignalScoreEvent): void => {
	if (!isSignalSource(event.source)) {
		return;
	}

	const confidence =
		typeof event.confidence === "number" && Number.isFinite(event.confidence)
			? event.confidence
			: 0;

	dashboardStore.setState((state) => ({
		...state,
		signalConfidences: {
			...state.signalConfidences,
			[event.source]: confidence,
		},
	}));
};

export const applyDecisionTrace = (trace: DecisionTraceEvent): void => {
	dashboardStore.setState((state) => ({
		...state,
		decisionTrace: trace,
	}));
};

export const buildFieldSnapshot = (
	state: DashboardState,
): FieldSnapshotEvent | undefined => {
	const symbols = Object.values(state.rows);

	if (symbols.length === 0 && !state.grid) {
		return undefined;
	}

	return {
		event: "field_snapshot",
		ts: new Date().toISOString(),
		symbol_count: state.symbolCount || symbols.length,
		field: state.aggregate ?? { re: 0, vort: 0, div: 0, turb: 0, visc: 0 },
		symbols,
		grid: state.grid,
	};
};

export const applyFieldRow = (row: FluidSymbolRow): void => {
	dashboardStore.setState((state) => ({
		...state,
		rows: {
			...state.rows,
			[row.symbol]: row,
		},
	}));
};

export const applyFieldAggregate = (
	symbolCount: number,
	field: FieldAggregate,
): void => {
	dashboardStore.setState((state) => ({
		...state,
		symbolCount,
		aggregate: field,
	}));
};

export const applyFieldGrid = (grid: FluidGridPayload): void => {
	dashboardStore.setState((state) => ({
		...state,
		grid,
	}));
};

export const applyFieldSnapshot = (snapshot: FieldSnapshotEvent): void => {
	const rows: Record<string, FluidSymbolRow> = {};

	for (const row of snapshot.symbols ?? []) {
		rows[row.symbol] = row;
	}

	dashboardStore.setState((state) => ({
		...state,
		rows,
		symbolCount: snapshot.symbol_count,
		aggregate: snapshot.field,
		grid: snapshot.grid ?? state.grid,
	}));
};

export const applyFluidDisplay = (display: FluidDisplayEvent): void => {
	dashboardStore.setState((state) => ({ ...state, fluidDisplay: display }));
};
