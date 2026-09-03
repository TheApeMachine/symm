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
import type { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";
import type { RegulatorFrame } from "#/providers/telemetry/telemetry/regulator-frame";
import type { DecisionT } from "#/providers/telemetry/telemetry/decision";
import type { StrategyFrame } from "#/providers/telemetry/telemetry/strategy-frame";
import type { TradeRecord } from "./types";

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

/*
createFrameBuffer wraps a ring in the FrameBuffer read surface. It is shared by
the flat and the keyed stores so both expose exactly the same reader API.
*/
export const createFrameBuffer = <T>(capacity = 50): FrameBuffer<T> => {
	const buffer = new RingBuffer<T>(capacity);

	return {
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
};

export const createFrameStore = <T>(capacity = 50) => {
	const state = createFrameBuffer<T>(capacity);

	return createStore(state, ({ setState }) => ({
		add: (frame: T) => {
			state.add(frame);
			setState((prev) => ({ ...prev, version: prev.version + 1 }));
		},
		reset: () => {
			state.clear();
			setState((prev) => ({ ...prev, version: 0 }));
		},
	}));
};

/*
createKeyedFrameStore keeps one ring per key instead of a single shared ring.
A shared ring cannot serve a per-symbol reader: with the whole universe
interleaving into 50 slots, any one symbol's newest frame is evicted long
before a consumer looks for it, so the reader either finds a foreign frame or
nothing at all. Partitioning by symbol keeps each symbol's history intact and
turns the lookup into a map hit rather than a scan.
*/
export interface KeyedFrames<T> {
	version: number;
	get(key: string): FrameBuffer<T> | undefined;
	getLast(key: string): T | undefined;
	keys(): string[];
}

export const createKeyedFrameStore = <T>(capacity = 50) => {
	const buffers = new Map<string, FrameBuffer<T>>();

	const bufferFor = (key: string): FrameBuffer<T> => {
		const existing = buffers.get(key);

		if (existing) {
			return existing;
		}

		const created = createFrameBuffer<T>(capacity);
		buffers.set(key, created);
		return created;
	};

	const state: KeyedFrames<T> = {
		version: 0,
		get: (key: string) => buffers.get(key),
		getLast: (key: string) => buffers.get(key)?.getLast(),
		keys: () => Array.from(buffers.keys()),
	};

	return createStore(state, ({ setState }) => ({
		add: (key: string, frame: T) => {
			bufferFor(key).add(frame);
			setState((prev) => ({ ...prev, version: prev.version + 1 }));
		},
		reset: () => {
			buffers.clear();
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

/*
Resonance transport status mirrors onlineStore but for the WebRTC data-channel
path that carries the predictive-coder resonance and diagnostics frames. It lets
the UI distinguish "the coder model is quiet" from "the telemetry transport is
down" — the former is normal model behavior, the latter is a transport failure
that deserves a visible indicator instead of a silently-blank panel.
*/
export type ResonanceTransportStatus = "OFFLINE" | "CONNECTING" | "ONLINE";

export const resonanceTransportStore =
	createStore<ResonanceTransportStatus>("OFFLINE");

/*
resonanceTransportDetail carries the current low-level connection state (e.g.
"failed", "disconnected", "new", "connecting") plus the raw RTCConnectionState,
so the UI can show a specific reason rather than a generic offline pill.
*/
export const resonanceTransportDetailStore = createStore<string>("");
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

/*
Measurements are partitioned by source AND symbol. Source alone is not enough:
one kernel emits for the whole universe, so a single ring per kernel holds ~640
symbols interleaved and a reader asking for one symbol's history gets a blend of
everyone else's instead — different magnitudes at unrelated epochs. Keying by
both is what makes a per-symbol series (a Hawkes intensity curve, say) actually
be that symbol's series.
*/
const measurementKey = (source: string, symbol: string) => `${source}\u0000${symbol}`;

export const getMeasurementStore = (source: string, symbol: string) => {
	const key = measurementKey(source, symbol);
	let store = measurementStores[key];
	if (!store) {
		store = createFrameStore<EnvelopeMeasurement>(50);
		measurementStores[key] = store;
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
	// Label is the measured symbol on a Measurement (Source names the kernel
	// that produced it), so the row itself says which symbol's ring it belongs
	// in — no need to thread the envelope key down here.
	getMeasurementStore(source, row.label() ?? "").actions.add(row);

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

	// Confidence is produced on the coder's very first step; it flows from the
	// first frame. Calibration stays visible on the artifact itself and
	// downstream surfaces weigh it, so this reducer must not suppress a feed
	// just because the head has not calibrated yet.
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
/*
Cognition is read per symbol on every surface that shows it, so it is keyed by
symbol rather than sharing one ring across the universe.
*/
export const cognitionStore = createKeyedFrameStore<EnvelopeCognition>(50);
export const categoryStore = createFrameStore<EnvelopeCategory>(50);
export const opportunityStore =
	createFrameStore<EnvelopeOpportunityCandidate>(50);
export const graphStore = createFrameStore<GraphFrame>(50);
export const strategyStore = createFrameStore<StrategyFrame>(50);

/*
decisionStore holds the latest decision per symbol, keyed by the symbol itself.

The decision surface is a board of live candidates: exactly one current
decision per symbol, replaced whenever that symbol is re-evaluated. A ring of
the last N *frames* could only reconstruct that by scanning and de-duplicating,
and — because a ring rotates — the frame position a row was pinned to silently
came to mean a different frame on every later tick. Rows then repainted from
foreign data and the list reshuffled under an open row.

Keying by symbol removes the problem rather than compensating for it: a row
looks its symbol up by name, so ring rotation cannot exist and a row's identity
is stable for as long as the symbol is being evaluated.

The stored value is the unpacked DecisionT, never the flatbuffer accessor. A
flatbuffer Decision is a mutable cursor into a shared buffer, so retaining one
would leave every stored symbol aliasing whichever decision was read last.
*/
export const decisionStore = createStore(
	{ version: 0, bySymbol: {} as Record<string, DecisionT> },
	({ setState }) => ({
		add: (decision: DecisionT) => {
			const symbol = decision.symbol;

			if (typeof symbol !== "string" || symbol === "") {
				return;
			}

			setState((prev) => ({
				version: prev.version + 1,
				bySymbol: { ...prev.bySymbol, [symbol]: decision },
			}));
		},
		reset: () => setState(() => ({ version: 0, bySymbol: {} })),
	}),
);
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
