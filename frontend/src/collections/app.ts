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
import type { BalancesFrame } from "#/providers/telemetry/telemetry/balances-frame";
import type { EnvelopeCategory } from "#/providers/telemetry/telemetry/envelope-category";
import type { EnvelopeCognition } from "#/providers/telemetry/telemetry/envelope-cognition";
import type { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import type { EnvelopeOpportunityCandidate } from "#/providers/telemetry/telemetry/envelope-opportunity-candidate";
import type { EnvelopeResonanceArtifact } from "#/providers/telemetry/telemetry/envelope-resonance-artifact";
import type { EnvelopeTickerData } from "#/providers/telemetry/telemetry/envelope-ticker-data";
import type { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";
import type { ErrorFrame } from "#/providers/telemetry/telemetry/error-frame";
import type { FluidPhaseFrame } from "#/providers/telemetry/telemetry/fluid-phase-frame";
import type { GraphFrame } from "#/providers/telemetry/telemetry/graph-frame";
import type { PerspectiveFrame } from "#/providers/telemetry/telemetry/perspective-frame";
import type { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";
import type { RegulatorFrame } from "#/providers/telemetry/telemetry/regulator-frame";
import type { StrategyFrame } from "#/providers/telemetry/telemetry/strategy-frame";

export const DEFAULT_KERNELS = [
	"correlation",
	"cvd",
	"depthflow",
	"derivatives",
	"hawkes",
	"leadlag",
	"liquidity",
	"morphology",
	"pumpdump",
	"sentiment",
	"toxicity",
];

export const DEFAULT_FOCUS_SYMBOL = "BTC/USD";

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

	return createStore(state, ({ setState }) => ({
		add: (frame: T) => {
			buffer.add(frame);
			setState((prev) => ({ ...prev, version: prev.version + 1 }));
		},
		reset: () => {
			buffer.clear();
			setState((prev) => ({ ...prev, version: 0 }));
		},
	}));
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
// tickCountStore is the monotonic engine clock (thesis.Tick), published on
// EnvelopeState.tick and read by the dashboard's tick counter. It is a single
// scalar, not a ring: the latest committed tick is the only value that matters.
export const tickCountStore = createStore<number>(0);

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
const measurementStores: Record<
	string,
	ReturnType<typeof createFrameStore<EnvelopeMeasurement>>
> = {};

export const measurementSourcesStore = createStore<string[]>([]);

export const getMeasurementStore = (source: string) => {
	let store = measurementStores[source];
	if (!store) {
		store = createFrameStore<EnvelopeMeasurement>(50);
		measurementStores[source] = store;
		measurementSourcesStore.setState((prev) =>
			prev.includes(source) ? prev : [...prev, source],
		);
	}
	return store;
};

/*
kernelReadingStores holds each kernel's usable readings — the SNR values that
were actually defined — separately from the raw measurement ring.

The two cannot be the same ring. Backend rows are sparse: a kernel emits a
measurement on every observation but only carries an SNR once its estimator has
a noise model, so a run of SNR-less rows is normal and says nothing about the
kernel's health. Deriving the trace from the raw ring made those rows
destructive — 50 of them evicted every real reading, and the row fell back to
Standby with a blue empty trace despite nothing having gone wrong.

This ring only ever advances on a real reading, so an update carrying no data
leaves the kernel exactly as it was. That is the honest reading of a sparse
update: no data means no change, never "the value is now nothing".
*/
const kernelReadingStores: Record<
	string,
	ReturnType<typeof createFrameStore<number>>
> = {};

export const getKernelReadingStore = (source: string) => {
	let store = kernelReadingStores[source];

	if (!store) {
		store = createFrameStore<number>(50);
		kernelReadingStores[source] = store;
	}

	return store;
};

export const addMeasurement = (source: string, row: EnvelopeMeasurement) => {
	getMeasurementStore(source).actions.add(row);

	// SNRDefined is the backend's own "this reading is real" flag (see
	// data.Measurement.Finalize): an undefined SNR is absent, not zero, so a row
	// without one contributes nothing rather than a fabricated reading.
	if (!row.snrDefined()) {
		return;
	}

	const snr = row.snr();

	if (Number.isFinite(snr)) {
		getKernelReadingStore(source).actions.add(snr);
	}
};

/*
kernelResonanceReadings mirrors kernelReadingStores for resonance, which is not
a measurement source: it carries a calibrated confidence rather than an SNR, and
its frames arrive for the whole cross-section rather than one focused symbol, so
its readings are kept per symbol.
*/
const kernelResonanceReadings: Record<
	string,
	ReturnType<typeof createFrameStore<number>>
> = {};

export const getResonanceReadingStore = (symbol: string) => {
	let store = kernelResonanceReadings[symbol];

	if (!store) {
		store = createFrameStore<number>(50);
		kernelResonanceReadings[symbol] = store;
	}

	return store;
};

export const addResonanceReading = (row: EnvelopeResonanceArtifact) => {
	resonanceArtifactStore.actions.add(row);

	if (!row.calibrated()) {
		return;
	}

	const confidence = row.confidence();

	if (Number.isFinite(confidence)) {
		getResonanceReadingStore(row.symbol() ?? "").actions.add(confidence);
	}
};

export const tickStore = createFrameStore<EnvelopeTickerData>(50);
export const regulatorStore = createFrameStore<RegulatorFrame>(50);
// resonanceArtifactStore carries types.Envelope.Resonance, which rides every
// envelope like every other measurement. It is the whole resonance surface:
// the predictive coder's per-layer states, latent vector, task-head quality,
// and forward curve, alongside the confidence/calibrated pair the kernel row
// reads. There is no second, curated resonance frame — the hub broadcasts the
// envelope as-is and never reshapes it per consumer.
export const resonanceArtifactStore =
	createFrameStore<EnvelopeResonanceArtifact>(50);
export const cognitionStore = createFrameStore<EnvelopeCognition>(50);
export const categoryStore = createFrameStore<EnvelopeCategory>(50);
export const opportunityStore =
	createFrameStore<EnvelopeOpportunityCandidate>(50);
export const graphStore = createFrameStore<GraphFrame>(50);
export const strategyStore = createFrameStore<StrategyFrame>(50);
export const perspectiveStore = createFrameStore<PerspectiveFrame>(50);
export const positionStore = createFrameStore<PositionsFrame>(50);
export const balanceStore = createFrameStore<BalancesFrame>(50);
export const equityStore = createFrameStore<EquityFrame>(50);

const fluidPhases: Record<string, RingBuffer<FluidFields>> = {};

export const fluidPhaseStore = createStore(fluidPhases, ({ setState }) => ({
	updatePhase: (name: string, fields: FluidFields) => {
		if (!fluidPhases[name]) fluidPhases[name] = new RingBuffer(50);
		fluidPhases[name].add(fields);
		setState((prev) => ({ ...prev }));
	},
}));

export const fluidFrameStore = createFrameStore<FluidPhaseFrame>(50);
export const errorFrameStore = createFrameStore<ErrorFrame>(50);

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
	},
	({ setState }) => ({
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
