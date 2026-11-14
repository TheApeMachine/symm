import { createAtom } from "@tanstack/react-store";
import { type Atom, createStore, type Store } from "@tanstack/store";
import type { ConfidenceFrame } from "./confidence";
import type {
	BookflowOutput,
	BookQualityOutput,
	CohortOutput,
	ConvictionOutput,
	Decision,
	DepthOutput,
	ExcitationOutcome,
	FlowOutput,
	IgnitionOutput,
	LagOutcome,
	LightTick,
	MorphologyOutput,
	Region,
	Resonance,
	Tokens,
} from "./envelope";
import type { PipelineFrame } from "./pipeline";
import { EMPTY_PIPELINE, mergeTopology } from "./pipeline";
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
	engine: createAtom<"booting" | "ready" | "error">("booting"),
	pipeline: createAtom<PipelineFrame>(EMPTY_PIPELINE),
	symbols: createAtom<string[]>([DEFAULT_FOCUS_SYMBOL]),
	error: createAtom<Record<string, unknown> | null>(null),
	tick: createAtom<LightTick | null>(null),
	balance: createAtom<Balance | null>(null),
	balances: createAtom<{
		virtual?: Balance | null;
		paper?: Balance | null;
		real?: Balance | null;
	}>({}),
	confidence: createAtom<ConfidenceFrame>({
		virtual: {
			active: false,
			armed: false,
			halted: false,
			closed: 0,
			wins: 0,
			win_rate: null,
			edge: null,
		},
		paper: {
			active: false,
			armed: false,
			halted: false,
			closed: 0,
			wins: 0,
			win_rate: null,
			edge: null,
		},
		real: {
			active: false,
			armed: false,
			halted: false,
			closed: 0,
			wins: 0,
			win_rate: null,
			edge: null,
		},
		eligible_resistance: 0.2,
		real_ceiling: false,
	}),
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
	// ListPrefix model/paths book (scope + thresholds) — not live sequencer paths.
	pathbook: createStore<{
		scope?: string;
		epoch?: string;
		thresholds?: Record<string, number>;
	}>({}),
	// ListPrefix closed-position chunks for history.
	closed: createStore<{
		scope?: string;
		entries?: unknown[];
	}>({}),
	positions: createStore<Record<string, Position>>({}),
	decisions: createStore<Record<string, Decision>>({}),
};

/*
direct maps hub envelope keys that cannot land on an atom/store as-is —
SciChart series, coalesced diag pulses, pipeline topology merges.
*/
export type DirectSink = (
	value: unknown,
	payload: Record<string, unknown>,
) => void;

export const direct: Record<string, DirectSink> = {};

export type OpenChoiceSink = {
	push: (
		marks: Record<string, Position>,
		opts?: { snapshot?: boolean; tier?: string },
	) => void;
};

let openChoice: OpenChoiceSink | null = null;

export const bindOpenChoice = (next: OpenChoiceSink | null) => {
	openChoice = next;
};

direct.positions = (value, payload) => {
	const marks = value as Record<string, Position>;
	const snapshot = payload.positions_snapshot === true;
	const tier = typeof payload.tier === "string" ? payload.tier : "";

	openChoice?.push(marks, { snapshot, tier });

	signals.positions.setState((prev: Record<string, Position> | null) => {
		if (!snapshot) return { ...(prev ?? {}), ...marks };

		const next: Record<string, Position> = {};
		for (const [id, pos] of Object.entries(prev ?? {})) {
			if (!pos) continue;
			const posTier =
				typeof pos.tier_key === "string" && pos.tier_key !== ""
					? pos.tier_key
					: "paper";
			if (!tier || posTier === tier) continue;
			next[id] = pos;
		}
		return { ...next, ...marks };
	});
};

direct.pipeline = (value) => {
	atoms.pipeline.set(
		mergeTopology(
			(atoms.pipeline.get() as PipelineFrame) ?? EMPTY_PIPELINE,
			(value as PipelineFrame) ?? EMPTY_PIPELINE,
		),
	);
};
