import { createStore } from "@tanstack/react-store";
import type { RingBuffer as RingBufferType } from "ring-buffer-ts";
import ringBufferPkg from "ring-buffer-ts";

// biome-ignore lint/suspicious/noExplicitAny: Because I'm Batman.
const RingBuffer = ((ringBufferPkg as any).RingBuffer ??
	// biome-ignore lint/suspicious/noExplicitAny: Because I'm Batman.
	(ringBufferPkg as any).default?.RingBuffer ??
	ringBufferPkg) as typeof RingBufferType;
type RingBuffer<T> = RingBufferType<T>;
export { RingBuffer };

import type { FluidFields } from "#/components/fluid-3d/wire";
import type { BacktestFrame } from "#/providers/telemetry/telemetry/backtest-frame";
import type { BalancesFrame } from "#/providers/telemetry/telemetry/balances-frame";
import type { CausalFrame } from "#/providers/telemetry/telemetry/causal-frame";
import type { CognitionFrame } from "#/providers/telemetry/telemetry/cognition-frame";
import type { DiagnosticQueue } from "#/providers/telemetry/telemetry/diagnostic-queue";
import type { DiagnosticsFrame } from "#/providers/telemetry/telemetry/diagnostics-frame";
import type { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";
import type { ErrorFrame } from "#/providers/telemetry/telemetry/error-frame";
import type { FluidPhaseFrame } from "#/providers/telemetry/telemetry/fluid-phase-frame";
import type { GraphFrame } from "#/providers/telemetry/telemetry/graph-frame";
import type { HindsightFrame } from "#/providers/telemetry/telemetry/hindsight-frame";
import type { Measurement } from "#/providers/telemetry/telemetry/measurement";
import type { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";
import type { RegulatorFrame } from "#/providers/telemetry/telemetry/regulator-frame";
import type { ResonanceFrame } from "#/providers/telemetry/telemetry/resonance-frame";
import type { StrategyFrame } from "#/providers/telemetry/telemetry/strategy-frame";
import type { TickFrame } from "#/providers/telemetry/telemetry/tick-frame";

export const DEFAULT_KERNELS = [
	"correlation",
	"cvd",
	"depthflow",
	"derivatives",
	"exhaustion",
	"hawkes",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
	"toxicity",
];

export const DEFAULT_FOCUS_SYMBOL = "BTC/USD";

export type BacktestCapture = {
	id: number;
	startedAt: string;
	endedAt?: string;
	frames: number;
};

/*
TradeRecord mirrors the JSON shape of wire.PositionT as returned by the hub's
GET /trades endpoint (broker.PositionStore.RecentTrades, backed by the
position_trades SQLite table) — the durable trade journal, independent of the
live positionStore ring buffer.
*/
export type TradeRecord = {
	status: string;
	decision?: Record<string, unknown> & {
		id?: string;
		thesisScore?: number;
		thesisConfidence?: number;
		causalIdentification?: string;
		allocationHaircut?: number;
		allocationHaircutReason?: string;
		adverseSelection?: string;
		expectedReturn?: string;
		expectedFees?: string;
		expectedSpread?: string;
		expectedImpact?: string;
		entryCost?: {
			bestAsk?: string;
			bestBid?: string;
			spread?: string;
			impact?: string;
			breakEven?: string;
			roundTripFees?: string;
		} | null;
		risk?: {
			riskDistance?: string;
			trailDistance?: string;
			armBuffer?: string;
			lockBuffer?: string;
			maxLoss?: string;
			minEdge?: string;
		} | null;
		trace?: {
			hypothesis?: string;
			recommendedAction?: string;
			graphSupports?: number;
			graphContradicts?: number;
		} | null;
	} | null;
	holding?: {
		symbol?: string;
		status?: string;
		entryAt?: number;
		exitAt?: number;
		entryPrice?: string;
		entryFee?: string;
		exitPrice?: string;
		exitFee?: string;
		pnl?: string;
		returnPct?: number;
		stoploss?: {
			status?: string;
			floor?: string;
			peak?: string;
			profitLine?: string;
			locked?: boolean;
			triggerReason?: string;
			triggerMark?: string;
			surgeArmed?: boolean;
		} | null;
	} | null;
};

export type HindsightBlocker = {
	key: string;
	category: string;
	label: string;
	source?: string;
	observed: number;
	target: number;
	hasTarget: boolean;
	gap: number;
	severity: number;
	explanation: string;
};

export type HindsightRecommendation = {
	key: string;
	kind: string;
	target: string;
	title: string;
	action: string;
	rationale: string;
	current: number;
	suggested: number;
	hasCurrent: boolean;
	hasSuggested: boolean;
	adjustment?: string;
	confidence: number;
	impactPct: number;
	occurrences: number;
	symbols: string[];
};

export type HindsightDiagnosis = {
	category: string;
	summary: string;
	evidenceQuality: number;
	evidenceStatus: string;
	blockers: HindsightBlocker[];
	recommendation: HindsightRecommendation | null;
};

export type HindsightRootCause = {
	category: string;
	impactPct: number;
	occurrences: number;
	symbols: string[];
};

export type HindsightSignal = {
	id: string;
	at: string | null;
	action?: string;
	reason?: string;
	cause?: string;
	graphScore: number;
	thesisScore: number;
	thesisConfidence?: number;
	thesisSupport?: number;
	thesisContradiction?: number;
	thesisConditions?: number;
	direction?: number;
	confidence?: number;
	admissionThreshold?: number;
	opportunity: boolean;
	opportunityType?: string;
	predictiveReady?: boolean;
	predictiveStatus?: string;
	alternatives: Record<string, number> | null;
};

export type HindsightLeg = {
	symbol: string;
	buyAt: string;
	buyPrice: number;
	sellAt: string;
	sellPrice: number;
	profitPct: number;
	grossProfitPct?: number;
	frictionPct?: number;
};

export type HindsightLoss = {
	symbol: string;
	decisionId: string;
	entryAt: string | null;
	exitAt: string | null;
	entryPrice: number;
	exitPrice: number;
	lossPerUnit: number;
	returnPct: number;
	grossPct: number;
	frictionPct: number;
	triggerReason?: string;
	diagnosis?: HindsightDiagnosis | null;
	signal: HindsightSignal;
	journal?: HindsightSignal[];
};

export type HindsightOpportunity = {
	leg: HindsightLeg;
	signal: HindsightSignal;
	journal?: HindsightSignal[];
	why?: string;
	diagnosis?: HindsightDiagnosis | null;
	captured: boolean;
	missed: boolean;
};

export type HindsightSymbol = {
	symbol: string;
	upboundPct: number;
	realizedPct: number;
	missedPct: number;
	lossPct?: number;
	legs: number;
	missedLegs: number;
	lossPositions?: number;
	opportunities: HindsightOpportunity[];
	losses?: HindsightLoss[];
};

export type HindsightReport = {
	captureId: number;
	status?: string;
	symbols: HindsightSymbol[];
	missedPct: number;
	upboundPct: number;
	missedLegs: number;
	totalLegs: number;
	realizedPct?: number;
	lossPct?: number;
	lossPositions?: number;
	valueCaptureRate?: number;
	legCaptureRate?: number;
	diagnosticCoverage?: number;
	rootCauses?: HindsightRootCause[];
	recommendations?: HindsightRecommendation[];
	lossRootCauses?: HindsightRootCause[];
	lossRecommendations?: HindsightRecommendation[];
};

export type BacktestState = {
	captureId: number | null;
	playing: boolean;
	position: string | null;
	startedAt: string | null;
	endedAt: string | null;
	rebooting: boolean;
	captures: BacktestCapture[];
	hindsight: HindsightReport | null;
};

export interface FrameBuffer<T> {
	version: number;
	getLast(): T | undefined;
	getFirst(): T | undefined;
	get(index: number): T | undefined;
	getFirstN(n: number): T[];
	getLastN(n: number): T[];
	findLast(predicate: (item: T) => boolean): T | undefined;
	toArray(): T[];
	getSize(): number;
	getBufferLength(): number;
	isFull(): boolean;
	isEmpty(): boolean;
	clear(): void;
	add(...items: T[]): void;
}

export const createFrameStore = <T>(capacity = 50) => {
	const buffer = new RingBuffer<T>(capacity);

	const state: FrameBuffer<T> = {
		version: 0,
		getLast: () => buffer.getLast(),
		getFirst: () => buffer.getFirst(),
		get: (index: number) => buffer.get(index),
		getFirstN: (n: number) => buffer.getFirstN(n),
		getLastN: (n: number) => buffer.getLastN(n),
		findLast: (predicate: (item: T) => boolean) => {
			const len = buffer.getBufferLength();
			for (let i = len - 1; i >= 0; i--) {
				const item = buffer.get(i);
				if (item !== undefined && predicate(item)) {
					return item;
				}
			}
			return undefined;
		},
		toArray: () => buffer.toArray(),
		getSize: () => buffer.getSize(),
		getBufferLength: () => buffer.getBufferLength(),
		isFull: () => buffer.isFull(),
		isEmpty: () => buffer.isEmpty(),
		clear: () => buffer.clear(),
		add: (...items: T[]) => buffer.add(...items),
	};

	return createStore(
		state,
		({ setState }) => ({
			add: (frame: T) => {
				buffer.add(frame);
				setState((prev) => ({ ...prev, version: prev.version + 1 }));
			},
			reset: () => {
				buffer.clear();
				setState((prev) => ({ ...prev, version: 0 }));
			},
		}),
	);
};

/*
Single-value Atomic Stores
*/
export const focusStore = createStore<string>(DEFAULT_FOCUS_SYMBOL);
export const onlineStore = createStore<"ONLINE" | "OFFLINE" | "CONNECTING">(
	"OFFLINE",
);
export const errorStore = createStore<
	Event | Error | Record<string, unknown> | null
>(null);
export const kernelDetailStore = createStore<string>("");
export const queryStore = createStore<string>("");
export const symbolsStore = createStore<string[]>([]);
export const observedSourcesStore = createStore<Set<string>>(new Set<string>());
export const startedAtMsStore = createStore<number | null>(null);
export const backtestStateStore = createStore<BacktestState>({
	captureId: null,
	playing: false,
	position: null,
	startedAt: null,
	endedAt: null,
	rebooting: false,
	captures: [],
	hindsight: null,
});

/*
Typed FlatBuffer Frame Stores (RingBuffer instances with TanStack Store actions)
*/

/*
One FrameStore per measurement source, not per source+symbol. The backend
already focus-gates every measurement to the currently focused symbol before
it ever reaches the browser (cmd/boot.go's ChannelMeasurements -> ChannelUI
wire drops anything not matching types.Focus()), so a symbol-keyed lookup on
the frontend was dead weight: it never needed to hold more than one symbol's
readings at a time, and its nested-map shape made every read and write pay
for a lookup that could never disambiguate anything. This mirrors every
other store in this file — a plain Record<string, FrameStore<T>> — and
avoids the whole "which source, which symbol, does either sub-map exist yet"
bookkeeping that came with the previous nested-map shape.

Sources are created lazily on first sight rather than pre-seeded from
DEFAULT_KERNELS, so a source the frontend doesn't yet know about by name
(e.g. a newly added kernel) still gets its own ring instead of being dropped.
*/
const measurementStores: Record<string, ReturnType<typeof createFrameStore<Measurement>>> = {};

export const measurementSourcesStore = createStore<string[]>([]);

export const getMeasurementStore = (source: string) => {
	let store = measurementStores[source];
	if (!store) {
		store = createFrameStore<Measurement>(50);
		measurementStores[source] = store;
		measurementSourcesStore.setState((prev) =>
			prev.includes(source) ? prev : [...prev, source],
		);
	}
	return store;
};

export const addMeasurement = (source: string, row: Measurement) => {
	getMeasurementStore(source).actions.add(row);
};

export const tickStore = createFrameStore<TickFrame>(50);
export const regulatorStore = createFrameStore<RegulatorFrame>(50);
export const resonanceStore = createFrameStore<ResonanceFrame>(50);
export const cognitionStore = createFrameStore<CognitionFrame>(50);
export const causalStore = createFrameStore<CausalFrame>(50);
export const graphStore = createFrameStore<GraphFrame>(50);
export const strategyStore = createFrameStore<StrategyFrame>(50);
export const positionStore = createFrameStore<PositionsFrame>(50);
export const balanceStore = createFrameStore<BalancesFrame>(50);
export const equityStore = createFrameStore<EquityFrame>(50);
export const diagnosticsFrameStore = createFrameStore<DiagnosticsFrame>(50);

const diagnosticQueues: Record<string, RingBuffer<DiagnosticQueue>> = {};

export const diagnosticStore = createStore(
	diagnosticQueues,
	({ setState }) => ({
		updateQueue: (name: string, row: DiagnosticQueue) => {
			if (!diagnosticQueues[name])
				diagnosticQueues[name] = new RingBuffer(1);
			diagnosticQueues[name].add(row);
			setState((prev) => ({ ...prev }));
		},
	}),
);

const fluidPhases: Record<string, RingBuffer<FluidFields>> = {};

export const fluidPhaseStore = createStore(
	fluidPhases,
	({ setState }) => ({
		updatePhase: (name: string, fields: FluidFields) => {
			if (!fluidPhases[name]) fluidPhases[name] = new RingBuffer(50);
			fluidPhases[name].add(fields);
			setState((prev) => ({ ...prev }));
		},
	}),
);

export const fluidFrameStore = createFrameStore<FluidPhaseFrame>(50);
export const errorFrameStore = createFrameStore<ErrorFrame>(50);
export const backtestStore = createFrameStore<BacktestFrame>(50);
export const hindsightStore = createFrameStore<HindsightFrame>(50);

/*
tradeHistoryStore holds the durable trade journal fetched from GET /trades.
Unlike positionStore (a 50-frame live-telemetry ring buffer that evicts closed
trades once enough newer frames from any symbol arrive), this is the full
persisted record from position_trades — it survives restarts and is not
bounded by tick volume.
*/
export const tradeHistoryStore = createStore<TradeRecord[]>([]);

/*
Backward compatibility appStore
*/
export const appStore = createStore(
	{
		online: false,
		error: null as Record<string, unknown> | null,
		focusSymbol: DEFAULT_FOCUS_SYMBOL,
		query: "",
		kernels: DEFAULT_KERNELS,
		observedSources: new Set<string>(),
		symbols: [] as string[],
		startedAtMs: null as number | null,
		backtest: {
			captureId: null,
			playing: false,
			position: null,
			startedAt: null,
			endedAt: null,
			rebooting: false,
			captures: [],
			hindsight: null,
		} as BacktestState,
	},
	({ setState }) => ({
		updateBacktest: (frame: Partial<BacktestState>) => {
			setState((prev) => ({
				...prev,
				backtest: { ...prev.backtest, ...frame },
			}));
			backtestStateStore.setState((prev) => ({ ...prev, ...frame }));
		},
		setBacktestCaptures: (captures: BacktestCapture[]) => {
			setState((prev) => ({
				...prev,
				backtest: { ...prev.backtest, captures },
			}));
			backtestStateStore.setState((prev) => ({ ...prev, captures }));
		},
		updateOnline: (online: boolean) => {
			setState((prev) => ({
				...prev,
				online,
				startedAtMs: online ? (prev.startedAtMs ?? Date.now()) : null,
			}));
			onlineStore.setState(() => (online ? "ONLINE" : "OFFLINE"));
		},
		updateError: (err: Record<string, unknown>) => {
			setState((prev) => ({
				...prev,
				error: err,
			}));
			errorStore.setState(() => err);
		},
		clearError: () => {
			setState((prev) => ({
				...prev,
				error: null,
			}));
			errorStore.setState(() => null);
		},
		updateFocusSymbol: (symbol: string) => {
			setState((prev) => ({
				...prev,
				focusSymbol: symbol,
			}));
			focusStore.setState(() => symbol);
		},
		updateQuery: (query: string) => {
			setState((prev) => ({
				...prev,
				query,
			}));
			queryStore.setState(() => query);
		},
		observeSymbols: (symbols: Iterable<string>) => {
			setState((prev) => {
				const merged = new Set(prev.symbols);
				let changed = false;

				for (const symbol of symbols) {
					if (symbol !== "" && !merged.has(symbol)) {
						merged.add(symbol);
						changed = true;
					}
				}

				if (changed) {
					const nextList = [...merged].sort();
					symbolsStore.setState(() => nextList);
					return { ...prev, symbols: nextList };
				}

				return prev;
			});
		},
		observeSources: (sources: Set<string>) => {
			setState((prev) => {
				if (sources.size === 0) {
					return prev;
				}

				const merged = new Set(prev.observedSources);
				let changed = false;

				for (const source of sources) {
					if (!merged.has(source)) {
						merged.add(source);
						changed = true;
					}
				}

				if (!changed) {
					return prev;
				}

				return {
					...prev,
					observedSources: merged,
				};
			});
		},
	}),
);
