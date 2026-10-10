import { createAtom } from "@tanstack/react-store";
import { createStore, type Store } from "@tanstack/store";
import { RingBuffer } from "./ring";
export { RingBuffer };

import type { DecisionT } from "#/providers/telemetry/telemetry/decision";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import type { HoldingT } from "#/providers/telemetry/telemetry/holding";
import type { PositionT } from "#/providers/telemetry/telemetry/position";

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
export const measurementSourcesAtom = createAtom<string[]>(DEFAULT_KERNELS);
export const kernelDetailAtom = createAtom<string>("cvd");

export const positionsAtom = createAtom<PositionT[]>([]);
// Round trips closed this session, each with the triggers of its sells.
export const closedPositionsAtom = createAtom<HoldingT[]>([]);
export const decisionsAtom = createAtom<DecisionT[]>([]);
export const resonanceStore = createStore<Record<string, RingBuffer<MeasurementT>>>({});

export const signals: Record<
	string,
	Store<Record<string, RingBuffer<MeasurementT>>>
> = {
	category: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	cognition: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	correlation: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	cvd: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	depthflow: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	derivatives: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	hawkes: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	leadlag: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	liquidity: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	morphology: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	pumpdump: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	sentiment: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	resonance: resonanceStore,
	toxicity: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	training: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
};

