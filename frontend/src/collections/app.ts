import { createAtom } from "@tanstack/react-store";
import { type Atom, createStore, type Store } from "@tanstack/store";
import type {
	BookflowOutput,
	BookQualityOutput,
	CohortOutput,
	ConvictionOutput,
	DepthOutput,
	ExcitationOutcome,
	FlowOutput,
	IgnitionOutput,
	LagOutcome,
	MorphologyOutput,
	Region,
	Resonance,
	Tokens,
} from "./envelope";
import type { Balance, Position } from "./types";

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

// biome-ignore lint/suspicious/noExplicitAny: TanStack Atom value types do not unify under one generic.
export const atoms: Record<string, Atom<any>> = {
	focus: createAtom<string>(DEFAULT_FOCUS_SYMBOL),
	route: createAtom<string>("dashboard"),
	focusMetric: createAtom<string>(""),
	online: createAtom<"ONLINE" | "OFFLINE" | "CONNECTING">("OFFLINE"),
	symbols: createAtom<string[]>([DEFAULT_FOCUS_SYMBOL]),
	error: createAtom<Record<string, unknown> | null>(null),
	tickCount: createAtom<number>(0),
	balance: createAtom<Balance | null>(null),
	phase: createAtom<string>("—"),
	candidates: createAtom<number>(0),
	positionCount: createAtom<number>(0),
	signalSources: createAtom<string[]>(DEFAULT_KERNELS),
	kernelDetail: createAtom<string>("cvd"),
};

// biome-ignore lint/suspicious/noExplicitAny: TanStack Atom value types do not unify under one generic.
export const signals: Record<string, Store<any>> = {
	correlation: createStore<Record<string, CohortOutput>>({}),
	cvd: createStore<Record<string, FlowOutput>>({}),
	depthflow: createStore<Record<string, BookflowOutput>>({}),
	hawkes: createStore<Record<string, ExcitationOutcome>>({}),
	leadlag: createStore<Record<string, LagOutcome>>({}),
	liquidity: createStore<Record<string, DepthOutput>>({}),
	morphology: createStore<Record<string, MorphologyOutput>>({}),
	pumpdump: createStore<Record<string, IgnitionOutput>>({}),
	sentiment: createStore<Record<string, ConvictionOutput>>({}),
	toxicity: createStore<Record<string, BookQualityOutput>>({}),
	resonance: createStore<Record<string, Resonance>>({}),
	regions: createStore<Record<string, Region[]>>({}),
	paths: createStore<Record<string, Tokens>>({}),
	positions: createStore<Record<string, Position>>({}),
};
