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

// Bracket lookup (`atoms["focus"]`) needs a wide bag type; atoms are still strongly typed at creation.
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

/*
Named aliases for the atoms record.
*/
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
One store per origin, keyed by the frame's scope. The tick's aggregate
workspace frame carries the kernels' layers as one batch: signalFor reads a
kernel's layers for a symbol straight off it, so every kernel consumer shares
the one ingest path instead of a store of its own.
*/
export const signals: Record<string, Store<Record<string, Value>>> = {
	workspace: createStore<Record<string, Value>>({}),
	category: createStore<Record<string, Value>>({}),
	resonance: createStore<Record<string, Value>>({}),
	manifold: createStore<Record<string, Value>>({}),
	training: createStore<Record<string, Value>>({}),
	positions: createStore<Record<string, Value>>({}),
};

/*
Named aliases for the signal stores.
*/
export const workspaceStore = signals.workspace;
export const resonanceStore = signals.resonance;
export const manifoldStore = signals.manifold;
export const trainingStore = signals.training;
export const positionsStore = signals.positions;

export const workspaceFrame = (): Value | undefined =>
	signals.workspace.state.aggregate;

/*
signalFor reads one kernel's layers for one symbol off the aggregate tick.
The Go Append addressed them <kernel>/<symbol>/<metric> below the frame
prefix (the symbol's own slash arrives escaped to "-", matching the desk's
per-lot position scopes), so the result maps each metric key to its decoded
payload — Append wrote one scalar array per key.
*/
export const signalFor = (
	kernel: string,
	symbol: string,
): Record<string, unknown> => {
	const frame = workspaceFrame();
	if (!frame) return {};

	const address = `${kernel}/${symbol}/`;
	const found: Record<string, unknown> = {};

	for (let i = 0; i < frame.index.length; i++) {
		const index = frame.index[i];
		if (!index.startsWith(`${frame.prefix}/${address}`)) continue;

		let payload: unknown;
		try {
			payload = JSON.parse(new TextDecoder().decode(frame.layers[i]));
		} catch {
			continue;
		}

		/*
		A kernel writes its metrics as one float array addressed by every
		metric name: cvd/BTC/USD/midpoint_log_return/flow_aligned_... The
		suffix splits into those names, one scalar array each, which is the
		shape readers look up by metric.
		*/
		const suffix = index.slice(`${frame.prefix}/${address}`.length);
		const names = suffix.split("/").filter(Boolean);
		if (
			Array.isArray(payload) &&
			names.length > 1 &&
			names.length === payload.length
		) {
			for (let n = 0; n < names.length; n++) {
				found[names[n]] = [payload[n]];
			}
			continue;
		}

		found[suffix] = payload;
	}

	return found;
};
