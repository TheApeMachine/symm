import { createAtom } from "@tanstack/react-store";
import { type Atom, createStore, type Store } from "@tanstack/store";

import { encodeJsonBytes, tryDecodeJsonBytes } from "#/lib/json-bytes";
import { Value } from "#/types/value";

export const DEFAULT_KERNELS = [
	"correlation",
	"depthflow",
	"hawkes",
	"liquidity",
	"pumpdump",
];

export const DEFAULT_FOCUS_SYMBOL = "BTC/USD";

/** Desk scope (BTC/USD) and escaped hub scope (BTC-USD) for layer lookups. */
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

// Bracket lookup (`atoms["focus"]`) needs a wide bag type; atoms are still strongly typed at creation.
// biome-ignore lint/suspicious/noExplicitAny: TanStack Atom value types do not unify under one generic.
export const atoms: Record<string, Atom<any>> = {
	focus: createAtom<string>(DEFAULT_FOCUS_SYMBOL),
	route: createAtom<string>("dashboard"),
	focusMetric: createAtom<string>(""),
	online: createAtom<"ONLINE" | "OFFLINE" | "CONNECTING">("OFFLINE"),
	symbols: createAtom<string[]>([DEFAULT_FOCUS_SYMBOL]),
	error: createAtom<Record<string, unknown> | null>(null),
	/** Canonical market tick index from workspace aggregate frame headers (not message count). */
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
	s3: createStore<Record<string, Value>>({}),
	paper: createStore<Record<string, Value>>({}),
	cognition: createStore<Record<string, Value>>({}),
};

/*
valueAtoms are hub origins that map to one atom, not a scope-indexed store.
*/
export const valueAtoms: Record<string, Atom<Value | null>> = {
	balance: atoms.balance,
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

const kernelLayerSources = new Set<string>(DEFAULT_KERNELS);

const symbolsOnAggregateCache = new WeakMap<Value, string[]>();

/*
symbolsOnAggregate lists desk symbols that appear on kernel or resonance
layers this tick (same path convention as logic/resonance.symbolsOnTick).
*/
export const symbolsOnAggregate = (frame: Value | undefined): string[] => {
	if (!frame) return [];

	const cached = symbolsOnAggregateCache.get(frame);
	if (cached) return cached;

	const root = frame.prefix ? `${frame.prefix}/` : "";
	const seen = new Set<string>();

	for (const index of frame.index) {
		if (root !== "" && !index.startsWith(root)) continue;

		const rel = root !== "" ? index.slice(root.length) : index;
		const parts = rel.split("/").filter(Boolean);
		if (parts.length < 2) continue;

		if (parts[0] === "resonance" && parts[1]) {
			const scope = parts[1];
			if (scope.includes("-")) {
				const dash = scope.indexOf("-");
				seen.add(`${scope.slice(0, dash)}/${scope.slice(dash + 1)}`);
			} else {
				seen.add(scope);
			}
			continue;
		}

		if (!kernelLayerSources.has(parts[0]) || parts.length < 3) continue;
		seen.add(`${parts[1]}/${parts[2]}`);
	}

	const list = [...seen].sort();
	symbolsOnAggregateCache.set(frame, list);
	return list;
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

/*
signalFor reads one kernel's layers for one symbol off the aggregate tick.
The Go Append addressed them <kernel>/<symbol>/<metric> below the frame
prefix (the symbol's own slash arrives escaped to "-", matching the desk's
per-lot position scopes), so the result maps each metric key to its decoded
payload — Append wrote one scalar array per key.
*/
type KernelReading = {
	metrics: Record<string, unknown>;
	bytes: Map<string, Uint8Array>;
};

type KernelCache = {
	index: string[];
	layers: Uint8Array[];
	size: number;
	byKey: Map<string, KernelReading>;
};

const kernelCache = new WeakMap<Value, KernelCache>();

const kernelCacheFor = (frame: Value): Map<string, KernelReading> => {
	const size = frame.index.length + frame.layers.length;
	const cached = kernelCache.get(frame);
	if (
		cached &&
		cached.index === frame.index &&
		cached.layers === frame.layers &&
		cached.size === size
	) {
		return cached.byKey;
	}

	const byKey = new Map<string, KernelReading>();
	kernelCache.set(frame, {
		index: frame.index,
		layers: frame.layers,
		size,
		byKey,
	});
	return byKey;
};

/*
readKernel scans one kernel's layers for one symbol once per frame. Later
readers of that pair, including every pin on the same kernel, reuse it.
*/
const ingestKernelLayer = (
	metrics: Record<string, unknown>,
	bytes: Map<string, Uint8Array>,
	suffix: string,
	payload: unknown,
	raw: Uint8Array,
) => {
	const names = suffix.split("/").filter(Boolean);

	if (Array.isArray(payload) && names.length > 0) {
		const count = Math.min(names.length, payload.length);
		for (let n = 0; n < count; n++) {
			const slice = [payload[n]];
			metrics[names[n]] = slice;
			bytes.set(names[n], encodeJsonBytes(slice));
		}
		return;
	}

	if (names.length === 1) {
		metrics[names[0]] = payload;
		bytes.set(names[0], raw);
		return;
	}

	metrics[suffix] = payload;
	bytes.set(suffix, raw);
};

const readKernel = (
	kernel: string,
	symbol: string,
	frame: Value,
): KernelReading => {
	const byKey = kernelCacheFor(frame);
	const cacheKey = `${kernel}/${symbol}`;
	const hit = byKey.get(cacheKey);
	if (hit) return hit;

	const metrics: Record<string, unknown> = {};
	const bytes = new Map<string, Uint8Array>();
	const root = frame.prefix ? `${frame.prefix}/` : "";

	for (let i = 0; i < frame.index.length; i++) {
		const index = frame.index[i];
		if (root !== "" && !index.startsWith(root)) continue;

		const marker = `/${kernel}/${symbol}/`;
		const searchFrom = frame.prefix ? frame.prefix.length : 0;
		const at = index.indexOf(marker, searchFrom);
		if (at < 0) continue;

		const payload = tryDecodeJsonBytes(frame.layers[i]);
		if (payload === undefined) continue;

		const suffix = index.slice(at + marker.length);
		ingestKernelLayer(metrics, bytes, suffix, payload, frame.layers[i]);
	}

	const reading = { metrics, bytes };
	byKey.set(cacheKey, reading);
	return reading;
};

const aggregateHasKernelLayers = (
	frame: Value,
	symbol: string,
): boolean => {
	for (const scope of symbolScopeAliases(symbol)) {
		for (const kernel of DEFAULT_KERNELS) {
			const marker = `/${kernel}/${scope}/`;
			if (frame.index.some((idx) => idx.includes(marker))) {
				return true;
			}
		}
	}
	return false;
};

const kernelQuerySticky = {
	focus: "",
	tick: 0n,
	symbol: "",
};

/*
kernelSymbolForAggregate keeps the dashboard focus when that symbol has
kernel layers on the tick; otherwise it uses the first symbol on the tick
that does (paper often trades alts while focus stays BTC/USD).
*/
export const kernelSymbolForAggregate = (
	focus: string,
	frame: Value | undefined,
): string => {
	if (!frame || !focus) return focus;

	if (aggregateHasKernelLayers(frame, focus)) {
		kernelQuerySticky.focus = focus;
		kernelQuerySticky.tick = frame.tick;
		kernelQuerySticky.symbol = focus;
		return focus;
	}

	if (
		kernelQuerySticky.focus === focus &&
		kernelQuerySticky.tick === frame.tick
	) {
		return kernelQuerySticky.symbol;
	}

	for (const symbol of symbolsOnAggregate(frame)) {
		if (aggregateHasKernelLayers(frame, symbol)) {
			kernelQuerySticky.focus = focus;
			kernelQuerySticky.tick = frame.tick;
			kernelQuerySticky.symbol = symbol;
			return symbol;
		}
	}

	return kernelQuerySticky.focus === focus
		? kernelQuerySticky.symbol
		: focus;
};

/** Kernels that publish their layers under the "aggregate" scope. */
const aggregateScopedKernels = new Set<string>([
	"correlation",
	"depthflow",
	"hawkes",
	"liquidity",
	"pumpdump",
]);

export const signalFor = (
	kernel: string,
	symbol: string,
	frame: Value | undefined = workspaceFrame(),
): Record<string, unknown> => {
	if (!frame) return {};
	for (const scope of symbolScopeAliases(symbol)) {
		const metrics = readKernel(kernel, scope, frame).metrics;
		if (Object.keys(metrics).length > 0) {
			return metrics;
		}
	}
	// Fallback: some kernels publish only under "aggregate", not per-symbol.
	if (aggregateScopedKernels.has(kernel)) {
		const metrics = readKernel(kernel, "aggregate", frame).metrics;
		if (Object.keys(metrics).length > 0) {
			return metrics;
		}
	}
	return {};
};

/*
kernelMeasurement folds one kernel's layers for a symbol off the aggregate
workspace tick into a synthetic Value the Metric Component can bind. The store
keys the batch under "aggregate", not per symbol.
*/
export const kernelMeasurement = (
	kernel: string,
	symbol: string,
	aggregate?: Value,
): Value | undefined => {
	const frame = aggregate ?? workspaceFrame();
	if (!frame) return undefined;

	let metrics: Record<string, unknown> = {};
	let bytes = new Map<string, Uint8Array>();

	for (const scope of symbolScopeAliases(symbol)) {
		const reading = readKernel(kernel, scope, frame);
		if (Object.keys(reading.metrics).length > 0) {
			metrics = reading.metrics;
			bytes = reading.bytes;
			break;
		}
	}

	// Fallback: some kernels publish only under "aggregate", not per-symbol.
	if (Object.keys(metrics).length === 0 && aggregateScopedKernels.has(kernel)) {
		const reading = readKernel(kernel, "aggregate", frame);
		if (Object.keys(reading.metrics).length > 0) {
			metrics = reading.metrics;
			bytes = reading.bytes;
		}
	}

	const keys = Object.keys(metrics);
	if (keys.length === 0) return undefined;

	const reading = new Value({
		scope: symbol,
		prefix: frame.prefix,
		tick: frame.tick,
		seqIdx: frame.seqIdx,
		timestamp: frame.timestamp,
		origin: kernel,
		role: frame.role,
	});

	for (const key of keys) {
		const layer = bytes.get(key);
		if (!layer) continue;
		reading.layers.push(layer);
		reading.index.push(`${reading.prefix}/${key}`);
	}

	return reading;
};
