import { createAtom } from "@tanstack/react-store";
import { type Atom, createStore, type Store } from "@tanstack/store";
import { RingBuffer } from "./ring";
export { RingBuffer };
export type * from "./types";

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
readPositionRows merges the desk's position frames into one row per symbol:
the merged "all" frame (open + closed, paper + virtual, live marks — the desk
broadcasts it every tick) with any newer per-event frame merged over it, so a
just-opened or just-closed lot shows its new state without waiting for the
next merged frame. Every position readout on the dashboard uses this.
*/
export const readPositionRows = (
	rings: Record<string, RingBuffer<Value> | undefined> | undefined,
): any[] => {
	// epoch -> insertion order lets newer frames overwrite older ones.
	const bySymbol = new Map<string, { at: bigint; row: any }>();

	for (const ring of Object.values(rings ?? {})) {
		if (!ring || ring.isEmpty()) continue;

		const latest = ring.get(ring.getBufferLength() - 1);
		if (!latest) continue;

		const rows = latest.lookup("positions") as any[] | undefined;
		if (!Array.isArray(rows)) continue;

		for (const row of rows) {
			if (!row || !row.symbol) continue;
			const key = row.id
				? `lot:${row.id}`
				: `${row.symbol}:${row.virtual ? "v" : "p"}`;
			const held = bySymbol.get(key);
			const at = BigInt(latest.epoch ?? 0);
			if (!held || at >= held.at) bySymbol.set(key, { at, row });
		}
	}

	return [...bySymbol.values()].map((entry) => entry.row);
};
