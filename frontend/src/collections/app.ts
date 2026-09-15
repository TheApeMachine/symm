import { type Atom, createAtom } from "@tanstack/react-store";
import { createStore, type Store } from "@tanstack/store";
import type { RingBuffer as RingBufferType } from "ring-buffer-ts";
import ringBufferPkg from "ring-buffer-ts";

// biome-ignore lint/suspicious/noExplicitAny: Because I'm Batman.
const RingBuffer = ((ringBufferPkg as any).RingBuffer ??
	// biome-ignore lint/suspicious/noExplicitAny: Because I'm Batman.
	(ringBufferPkg as any).default?.RingBuffer ??
	ringBufferPkg) as typeof RingBufferType;
type RingBuffer<T> = RingBufferType<T>;
export { RingBuffer };

import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import type { ResonanceT } from "#/providers/telemetry/telemetry/resonance";

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

export type StateAtom<T> = Atom<T> & {
	readonly state: T;
	setState: (updater: T | ((prev: T) => T)) => void;
};

export const createAtomic = <T>(initialValue: T): StateAtom<T> => {
	const a = createAtom<T>(initialValue);
	Object.defineProperty(a, "state", {
		get() {
			return a.get();
		},
	});
	(a as StateAtom<T>).setState = (updater) => {
		if (typeof updater === "function") {
			a.set(updater as (prevVal: T) => T);
			return;
		}

		a.set(updater);
	};
	return a as StateAtom<T>;
};

/*
Single-value Atoms
*/
export const focusAtom = createAtomic<string>(DEFAULT_FOCUS_SYMBOL);
export const focusStore = focusAtom;

export const routeAtom = createAtomic<string>("dashboard");
export const routeStore = routeAtom;

export const focusMetricAtom = createAtomic<string>("");
export const focusMetric = focusMetricAtom;

export const onlineAtom = createAtomic<"ONLINE" | "OFFLINE" | "CONNECTING">(
	"OFFLINE",
);
export const onlineStore = onlineAtom;

export const symbolsAtom = createAtomic<string[]>([DEFAULT_FOCUS_SYMBOL]);
export const symbolsStore = symbolsAtom;

export const errorAtom = createAtomic<Record<string, unknown> | null>(null);
export const errorStore = errorAtom;

export const cashAtom = createAtomic<string>("");
export const cashStore = cashAtom;

export const unrealizedAtom = createAtomic<string>("");
export const unrealizedStore = unrealizedAtom;

export const equityAtom = createAtomic<string>("");
export const equityStore = equityAtom;

export const updateEquity = (
	cash?: string | null,
	unrealized?: string | null,
	equity?: string | null,
) => {
	if (cash !== null && cash !== undefined && cash.trim() !== "") {
		cashAtom.set(cash);
	}
	if (
		unrealized !== null &&
		unrealized !== undefined &&
		unrealized.trim() !== ""
	) {
		unrealizedAtom.set(unrealized);
	}
	if (equity !== null && equity !== undefined && equity.trim() !== "") {
		equityAtom.set(equity);
	}
};

export const observeSymbols = (symbols: Iterable<string>) => {
	const current = new Set(symbolsAtom.get());
	let changed = false;

	for (const sym of symbols) {
		if (sym !== "" && !current.has(sym)) {
			current.add(sym);
			changed = true;
		}
	}

	if (changed) {
		symbolsAtom.set([...current].sort());
	}
};

export const clockAtom = createAtomic<number | null>(null);
export const clockStore = clockAtom;

export const updateClock = (at: number | bigint | null | undefined) => {
	if (at === null || at === undefined) return;
	const val =
		typeof at === "bigint"
			? at > 1000000000000000n
				? Number(at / 1000000n)
				: Number(at)
			: at;
	if (Number.isFinite(val) && val > 0) {
		clockAtom.set(val);
	}
};

export const tickCountAtom = createAtomic<number>(0);
export const tickCountStore = tickCountAtom;
export const tickStore = tickCountAtom;

export const phaseAtom = createAtomic<string>("—");
export const phaseStore = phaseAtom;

export const candidatesAtom = createAtomic<number>(0);
export const candidatesStore = candidatesAtom;

export const positionCountAtom = createAtomic<number>(0);
export const positionCountStore = positionCountAtom;

export const measurementSourcesAtom = createAtomic<string[]>(DEFAULT_KERNELS);
export const measurementSourcesStore = measurementSourcesAtom;

export const kernelDetailAtom = createAtomic<string>("cvd");
export const kernelDetailStore = kernelDetailAtom;

export const resonanceStore = createStore<
	Record<string, RingBuffer<MeasurementT | ResonanceT>>
>({});
export const resonanceArtifactStore = resonanceStore;

export const categoryStore = createStore<
	Record<string, RingBuffer<MeasurementT>>
>({});
export const trainingStore = createStore<
	Record<string, RingBuffer<MeasurementT>>
>({});

export const signals: Record<
	string,
	Store<Record<string, RingBuffer<MeasurementT>>>
> = {
	category: categoryStore,
	correlation: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	cvd: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	depthflow: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	derivatives: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	hawkes: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	leadlag: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	liquidity: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	morphology: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	pumpdump: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	resonance: resonanceStore as any,
	sentiment: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	toxicity: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	training: trainingStore,
};

export type FrameBuffer<T> = RingBuffer<T>;

const createFallbackFrameStore = () =>
	createStore<any>({
		findLast: () => null,
		getLast: () => null,
		getFirst: () => null,
		toArray: () => [],
		rowsLength: () => 0,
		rows: () => null,
	});

export const strategyStore = createFallbackFrameStore();
export const positionStore = createFallbackFrameStore();
export const manifoldStore = createStore<any>({});
export const tradeHistoryStore = createStore<any[]>([]);

export const cognitionStore = createStore<Record<string, any>>({});
Object.defineProperty(cognitionStore.state, "getLast", {
	value: (symbol: string) => {
		const val = (cognitionStore.state as any)?.[symbol];
		if (val && typeof val.getLast === "function") {
			return val.getLast();
		}
		return val;
	},
	writable: true,
	configurable: true,
});

export type MeasurementStore = Store<RingBuffer<MeasurementT>> & {
	actions: {
		add: (item: MeasurementT) => void;
	};
	add: (item: MeasurementT) => void;
};

export const MAX_CACHED_SYMBOLS = 64;

const measurementStoreCache: Record<
	string,
	Store<RingBuffer<MeasurementT>>
> = {};
const measurementSubscribers: Record<string, () => void> = {};

const resonanceReadingStoreCache: Record<
	string,
	Store<RingBuffer<MeasurementT | ResonanceT>>
> = {};
const resonanceSubscribers: Record<string, () => void> = {};

const symbolAccessTimes = new Map<string, number>();

export const getCachedSymbolCount = (): number => symbolAccessTimes.size;

export const touchSymbol = (symbol: string) => {
	if (!symbol) return;
	symbolAccessTimes.set(symbol, Date.now());

	if (symbolAccessTimes.size > MAX_CACHED_SYMBOLS) {
		const focused = focusAtom.get();
		let oldestSymbol: string | null = null;
		let oldestTime = Number.POSITIVE_INFINITY;

		for (const [sym, at] of symbolAccessTimes.entries()) {
			if (sym === focused || sym === DEFAULT_FOCUS_SYMBOL) {
				continue;
			}
			if (at < oldestTime) {
				oldestTime = at;
				oldestSymbol = sym;
			}
		}

		if (oldestSymbol) {
			evictSymbol(oldestSymbol);
		}
	}
};

export const evictSymbol = (symbol: string) => {
	if (!symbol) return;

	symbolAccessTimes.delete(symbol);

	for (const source of Object.keys(signals)) {
		const key = `${source}::${symbol}`;
		if (measurementSubscribers[key]) {
			measurementSubscribers[key]();
			delete measurementSubscribers[key];
		}
		delete measurementStoreCache[key];

		const signalStore = signals[source];
		if (signalStore?.state?.[symbol]) {
			delete signalStore.state[symbol];
		}
	}

	if (resonanceSubscribers[symbol]) {
		resonanceSubscribers[symbol]();
		delete resonanceSubscribers[symbol];
	}
	delete resonanceReadingStoreCache[symbol];
	if (resonanceStore.state[symbol]) {
		delete resonanceStore.state[symbol];
	}

	if ((cognitionStore.state as any)?.[symbol]) {
		delete (cognitionStore.state as any)[symbol];
	}

	const current = symbolsAtom.get();
	if (
		current.includes(symbol) &&
		symbol !== DEFAULT_FOCUS_SYMBOL &&
		symbol !== focusAtom.get()
	) {
		symbolsAtom.set(current.filter((s) => s !== symbol));
	}
};

export const evictStaleSymbols = (maxAgeMs = 60_000) => {
	const now = Date.now();
	const focused = focusAtom.get();
	const toEvict: string[] = [];

	for (const [sym, at] of symbolAccessTimes.entries()) {
		if (sym === focused || sym === DEFAULT_FOCUS_SYMBOL) {
			continue;
		}
		if (now - at >= maxAgeMs) {
			toEvict.push(sym);
		}
	}

	for (const sym of toEvict) {
		evictSymbol(sym);
	}
};

export const getMeasurementStore = (
	source: string,
	symbol: string,
): MeasurementStore => {
	touchSymbol(symbol);
	const key = `${source}::${symbol}`;
	let store = measurementStoreCache[key];
	if (!store) {
		const signalStore = signals[source];
		const getRing = () => {
			if (!signalStore) return new RingBuffer<MeasurementT>(50);
			let ring = signalStore.state[symbol];
			if (!ring) {
				ring = new RingBuffer<MeasurementT>(50);
				signalStore.state[symbol] = ring;
			}
			return ring;
		};

		store = createStore(getRing());
		measurementStoreCache[key] = store;

		if (signalStore) {
			const sub = signalStore.subscribe((state) => {
				const ring = state[symbol];
				if (ring) {
					store.setState(() => ring);
				}
			});
			measurementSubscribers[key] = () => sub.unsubscribe();
		}
	}
	return Object.assign(store, {
		actions: {
			add: (item: any) => {
				store.state.add(item);
				store.setState((prev) => prev);
			},
		},
		add: (item: any) => {
			store.state.add(item);
			store.setState((prev) => prev);
		},
	}) as MeasurementStore;
};

export const getResonanceReadingStore = (symbol: string) => {
	touchSymbol(symbol);
	let store = resonanceReadingStoreCache[symbol];
	if (!store) {
		const getRing = () => {
			let ring = resonanceStore.state[symbol];
			if (!ring) {
				ring = new RingBuffer<MeasurementT | ResonanceT>(50);
				resonanceStore.state[symbol] = ring;
			}
			return ring;
		};
		store = createStore(getRing());
		resonanceReadingStoreCache[symbol] = store;

		const sub = resonanceStore.subscribe((state) => {
			const ring = state[symbol];
			if (ring) {
				store.setState(() => ring);
			}
		});
		resonanceSubscribers[symbol] = () => sub.unsubscribe();
	}
	return store;
};
