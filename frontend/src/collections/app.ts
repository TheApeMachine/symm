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

export type SignalsRegistry = Record<
	string,
	Store<Record<string, RingBuffer<MeasurementT>>>
>;

export const signals: SignalsRegistry = {
	category: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	correlation: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	cvd: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	depthflow: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	derivatives: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	hawkes: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	leadlag: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	liquidity: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	morphology: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	pumpdump: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	resonance: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	sentiment: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	toxicity: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	training: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	strategy: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	position: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	manifold: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	trades: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
	cognition: createStore<Record<string, RingBuffer<MeasurementT>>>({}),
};
