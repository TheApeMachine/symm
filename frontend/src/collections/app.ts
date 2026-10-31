import { createAtom } from "@tanstack/react-store";
import { type Atom, createStore, type Store } from "@tanstack/store";

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

/** Desk scope (BTC/USD) and escaped hub scope (BTC-USD). */
export const symbolScopeAliases = (symbol: string): string[] => {
	if (!symbol) return [];
	const aliases = new Set<string>([symbol]);
	if (symbol.includes("/")) {
		aliases.add(symbol.replace(/\//g, "-"));
	} else {
		const dash = symbol.indexOf("-");
		if (dash > 0) {
			aliases.add(`${symbol.slice(0, dash)}/${symbol.slice(dash + 1)}`);
		}
	}
	return [...aliases];
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

// biome-ignore lint/suspicious/noExplicitAny: TanStack Atom value types do not unify under one generic.
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

export const focusAtom = atoms.focus;
export const routeAtom = atoms.route;
export const focusMetricAtom = atoms.focusMetric;
export const onlineAtom = atoms.online;
export const symbolsAtom = atoms.symbols;
export const errorAtom = atoms.error;
export const tickCountAtom = atoms.tickCount;
export const balanceAtom = atoms.balance;
export const phaseAtom = atoms.phase;
export const candidatesAtom = atoms.candidates;
export const positionCountAtom = atoms.positionCount;
export const signalSourcesAtom = atoms.signalSources;
export const kernelDetailAtom = atoms.kernelDetail;

/*
Hub frames land in signals[origin][scope]. Websocket creates missing origins
on first sight; consumers subscribe to the origin store they care about.
*/
export const signals: Record<string, Store<Record<string, Value>>> = {
	workspace: createStore<Record<string, Value>>({}),
	resonance: createStore<Record<string, Value>>({}),
	training: createStore<Record<string, Value>>({}),
	positions: createStore<Record<string, Value>>({}),
	s3: createStore<Record<string, Value>>({}),
	paper: createStore<Record<string, Value>>({}),
	cognition: createStore<Record<string, Value>>({}),
	grid: createStore<Record<string, Value>>({}),
	sequencer: createStore<Record<string, Value>>({}),
	train: createStore<Record<string, Value>>({}),
};

export const valueAtoms: Record<string, Atom<Value | null>> = {
	balance: atoms.balance,
};

export const workspaceStore = signals.workspace;
export const resonanceStore = signals.resonance;
export const trainingStore = signals.training;
export const positionsStore = signals.positions;

export const workspaceFrame = (): Value | undefined =>
	signals.workspace.state.aggregate;

export const ensureSignalStore = (
	origin: string,
): Store<Record<string, Value>> => {
	const existing = signals[origin];
	if (existing) return existing;

	const created = createStore<Record<string, Value>>({});
	signals[origin] = created;
	return created;
};

/*
latest returns the current frame for origin+scope (slash/dash aliases).
*/
export const latest = (
	origin: string,
	scope: string,
): Value | undefined => {
	const store = signals[origin];
	if (!store || !scope) return undefined;

	for (const alias of symbolScopeAliases(scope)) {
		const hit = store.state[alias];
		if (hit) return hit;
	}

	return undefined;
};

export const resonanceFrameForSymbol = (
	state: Record<string, Value>,
	symbol: string,
): Value | undefined => {
	for (const scope of symbolScopeAliases(symbol)) {
		const hit = state[scope];
		if (hit) return hit;
	}
	return undefined;
};
