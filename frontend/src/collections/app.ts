import { createAtom } from "@tanstack/react-store";
import { createStore, type Store } from "@tanstack/store";
import { RingBuffer } from "./ring";
export { RingBuffer };

import { Value } from "#/types/value";
export type { Metric, NamedNumber, Tag } from "#/types/value";
export { Value };

export const DEFAULT_KERNELS = [
	"correlation",
	"cvd",
	"depthflow",
	"hawkes",
	"leadlag",
	"liquidity",
	"morphology",
	"pumpdump",
	"sentiment",
	"toxicity",
];

export const DEFAULT_FOCUS_SYMBOL = "BTC/USD";

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

export const clockAtom = createAtom<number | null>(null);

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

export const focusAtom = createAtom<string>(DEFAULT_FOCUS_SYMBOL);
export const routeAtom = createAtom<string>("dashboard");
export const focusMetricAtom = createAtom<string>("");
export const onlineAtom = createAtom<"ONLINE" | "OFFLINE" | "CONNECTING">(
	"OFFLINE",
);
export const symbolsAtom = createAtom<string[]>([DEFAULT_FOCUS_SYMBOL]);
export const errorAtom = createAtom<Record<string, unknown> | null>(null);
export const cashAtom = createAtom<string>("");
export const unrealizedAtom = createAtom<string>("");
export const equityAtom = createAtom<string>("");
export const tickCountAtom = createAtom<number>(0);
export const phaseAtom = createAtom<string>("—");
export const candidatesAtom = createAtom<number>(0);
export const positionCountAtom = createAtom<number>(0);
export const signalSourcesAtom = createAtom<string[]>(DEFAULT_KERNELS);
export const kernelDetailAtom = createAtom<string>("cvd");

import type { DecisionT, HoldingT, PositionT } from "#/collections/types";

export const positionsAtom = createAtom<PositionT[]>([]);
// Round trips closed this session, each with the triggers of its sells.
export const closedPositionsAtom = createAtom<HoldingT[]>([]);
export const decisionsAtom = createAtom<DecisionT[]>([]);
export const resonanceStore = createStore<Record<string, RingBuffer<Value>>>(
	{},
);

export const signals: Record<
	string,
	Store<Record<string, RingBuffer<Value>>>
> = {
	category: createStore<Record<string, RingBuffer<Value>>>({}),
	correlation: createStore<Record<string, RingBuffer<Value>>>({}),
	cvd: createStore<Record<string, RingBuffer<Value>>>({}),
	depthflow: createStore<Record<string, RingBuffer<Value>>>({}),
	hawkes: createStore<Record<string, RingBuffer<Value>>>({}),
	leadlag: createStore<Record<string, RingBuffer<Value>>>({}),
	liquidity: createStore<Record<string, RingBuffer<Value>>>({}),
	morphology: createStore<Record<string, RingBuffer<Value>>>({}),
	pumpdump: createStore<Record<string, RingBuffer<Value>>>({}),
	sentiment: createStore<Record<string, RingBuffer<Value>>>({}),
	resonance: resonanceStore,
	toxicity: createStore<Record<string, RingBuffer<Value>>>({}),
	training: createStore<Record<string, RingBuffer<Value>>>({}),
	manifold: createStore<Record<string, RingBuffer<Value>>>({}),
};
