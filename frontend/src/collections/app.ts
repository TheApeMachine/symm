import { createAtom } from "@tanstack/react-store";
import { type Atom, createStore, type Store } from "@tanstack/store";
import { RingBuffer } from "./ring";
export { RingBuffer };

import type { Value } from "#/types/value";

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

export const atoms: Record<string, Atom<any>> = {
	focus: createAtom<string>(DEFAULT_FOCUS_SYMBOL),
	route: createAtom<string>("dashboard"),
	focusMetric: createAtom<string>(""),
	online: createAtom<"ONLINE" | "OFFLINE" | "CONNECTING">("OFFLINE"),
	symbols: createAtom<string[]>([DEFAULT_FOCUS_SYMBOL]),
	error: createAtom<Record<string, unknown> | null>(null),
	tickCount: createAtom<number>(0),
	balance: createAtom<Value | null>(null),
	phase: createAtom<string>("—"),
	candidates: createAtom<number>(0),
	positionCount: createAtom<number>(0),
	signalSources: createAtom<string[]>(DEFAULT_KERNELS),
	kernelDetail: createAtom<string>("cvd"),
};

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
	resonance: createStore<Record<string, RingBuffer<Value>>>({}),
	toxicity: createStore<Record<string, RingBuffer<Value>>>({}),
	training: createStore<Record<string, RingBuffer<Value>>>({}),
	manifold: createStore<Record<string, RingBuffer<Value>>>({}),
	positions: createStore<Record<string, RingBuffer<Value>>>({}),
};

/*
positionRows reads the desk's latest frame: the ring keyed by the frame's
scope holds the merged positions broadcast (one per tick, all lots), and
lookup("positions") decodes the rows straight off the Value.
*/
export const positionRows = (
	store: Store<Record<string, RingBuffer<Value>>>,
	scope: string,
): any[] => {
	const ring = store.state[scope];
	if (!ring || ring.isEmpty()) return [];

	const latest = ring.getLast();
	if (!latest) return [];

	const rows = latest.lookup("positions") as any[] | undefined;
	return Array.isArray(rows) ? rows : [];
};
