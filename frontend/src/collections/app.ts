import { createAtom } from "@tanstack/react-store";
import { createStore, type Store } from "@tanstack/store";
import { RingBuffer } from "./ring";
export { RingBuffer };

import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

export const SIGNALS = [
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
Single-value Atoms
*/
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

export const tickCountAtom = createAtom<number>(0);
export const phaseAtom = createAtom<string>("—");
export const candidatesAtom = createAtom<number>(0);
export const positionCountAtom = createAtom<number>(0);
export const measurementSourcesAtom = createAtom<string[]>(SIGNALS);
export const kernelDetailAtom = createAtom<string>("cvd");

export type SignalsRegistry = Record<string, Store<Record<string, any>>>;

export const signals: SignalsRegistry = {
	category: createStore({}),
	correlation: createStore({}),
	cvd: createStore({}),
	depthflow: createStore({}),
	derivatives: createStore({}),
	hawkes: createStore({}),
	leadlag: createStore({}),
	liquidity: createStore({}),
	morphology: createStore({}),
	pumpdump: createStore({}),
	resonance: createStore({}),
	sentiment: createStore({}),
	toxicity: createStore({}),
	training: createStore({}),
	strategy: createStore({}),
	position: createStore({}),
	manifold: createStore({}),
	trades: createStore({}),
	cognition: createStore({}),
};

export const evictSymbol = (symbol: string) => {
	const current = new Set(symbolsAtom.get());
	current.delete(symbol);
	symbolsAtom.set(Array.from(current));
};

export const evictStaleSymbols = () => {
	const current = new Set(symbolsAtom.get());
	symbolsAtom.set(Array.from(current));
};

export const updateEquity = (cash: string, unrealized: string, equity: string) => { cashAtom.set(cash); unrealizedAtom.set(unrealized); equityAtom.set(equity); };
