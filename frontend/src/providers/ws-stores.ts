import { createStore } from "@tanstack/store";
import { useSelector } from "@tanstack/react-store";
import { useLayoutEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { latestOf, latestValues } from "#/collections/circular";
import {
	DECISION_HISTORY_LIMIT,
	decisionStore,
	latestStrategyDecisions,
} from "#/collections/decisions";
import { createKeyedStore } from "#/collections/store";
import type {
	CausalFrame,
	CognitiveReading,
	Measurement,
	Position,
	ResonanceFrame,
	TickFrame,
} from "#/collections/types";
import { paintTerminalResonanceChart } from "#/components/charts/resonance";
import type { JSONSerializable } from "#/components/ui/paint";
import type { Decision } from "#/types/thesis";

const RETAINED_FRAME_LIMIT = 1;
export const JOURNAL_ENTRY_LIMIT = DECISION_HISTORY_LIMIT;

/*
Typed wire frames. A scalar frame is one decoded table; a keyed frame retains the
latest row per identity so a scoped surface keeps its row across sparse batches.
*/
export type EquityFrame = {
	cash: number | string;
	unrealized: number | string;
	equity: number | string;
};

export type StrategyFrame = {
	outcome?: string;
	evaluated?: string;
	decisions: Decision[];
};

type ScalarState<T> = T | null;

export const tickStore = createStore<ScalarState<TickFrame>>(null);
export const equityStore = createStore<ScalarState<EquityFrame>>(null);
export const strategyStore = createStore<ScalarState<StrategyFrame>>(null);
export const activityStore = createStore<ScalarState<Record<string, string>>>(null);
export const causalStore = createStore<ScalarState<CausalFrame[]>>(null);
export const graphStore = createStore<ScalarState<Record<string, unknown>>>(null);
export const regulatorStore = createStore<ScalarState<Record<string, unknown>>>(null);
export const resonanceFocusStore = createStore<ScalarState<ResonanceFrame>>(null);

export const measurementsStore = createKeyedStore<Measurement>()(
	"measurements",
	RETAINED_FRAME_LIMIT,
	(row) => measurementIdentity(row),
);
export const cognitionStore = createKeyedStore<CognitiveReading>()(
	"cognition",
	RETAINED_FRAME_LIMIT,
	(row) => symbolIdentity(row),
);
export const resonanceStore = createKeyedStore<ResonanceFrame>()(
	"resonance",
	RETAINED_FRAME_LIMIT,
	(row) => symbolIdentity(row),
);
export const positionsStore = createKeyedStore<Position>()(
	"positions",
	RETAINED_FRAME_LIMIT,
	(row) => positionIdentity(row),
);
export const journalStore = createKeyedStore<Position>()("journal", JOURNAL_ENTRY_LIMIT);

const symbolIdentity = (row: { symbol?: unknown }): string =>
	typeof row.symbol === "string" ? row.symbol : "";

export const measurementIdentity = (row: Pick<Measurement, "source" | "symbol">): string =>
	typeof row.source === "string" && typeof row.symbol === "string"
		? `${row.source}\u0000${row.symbol}`
		: "";

const positionIdentity = (row: Position): string =>
	typeof row.holding?.symbol === "string" ? row.holding.symbol : "";

/*
Narrow the heterogeneous decoder row into the concrete store type. These are the
only casts in the whole pipeline — one per key, at the boundary.
*/
const toMeasurement = (value: JSONSerializable): Measurement[] => {
	const rows = Array.isArray(value) ? value : [value];

	return rows.filter(
		(row): row is Measurement =>
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			typeof (row as { source?: unknown }).source === "string" &&
			typeof (row as { symbol?: unknown }).symbol === "string",
	) as Measurement[];
};

const toCognitive = (value: JSONSerializable): CognitiveReading[] => {
	const rows = Array.isArray(value) ? value : Object.values(value ?? {});

	return rows.filter(
		(row): row is CognitiveReading =>
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			typeof (row as { symbol?: unknown }).symbol === "string",
	) as CognitiveReading[];
};

const toResonance = (value: JSONSerializable): ResonanceFrame[] => {
	const rows = Array.isArray(value) ? value : Object.values(value ?? {});

	return rows.filter(
		(row): row is ResonanceFrame =>
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			typeof (row as { symbol?: unknown }).symbol === "string",
	) as ResonanceFrame[];
};

const toPositions = (value: JSONSerializable): Position[] => {
	const rows = Array.isArray(value) ? value : [value];
	const output: Position[] = [];

	for (const row of rows) {
		const isPosition =
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			(row as { holding?: { symbol?: unknown } }).holding !== null &&
			typeof (row as { holding?: { symbol?: unknown } }).holding?.symbol ===
				"string";

		if (isPosition) {
			output.push(row as unknown as Position);
		}
	}

	return output;
};

const positionIsTerminal = (position: Position): boolean => {
	const status = position.status;

	return (
		status === "canceled" ||
		status === "closed" ||
		status === "error" ||
		status === "expired" ||
		status === "fatal" ||
		status === "partial_cancelled" ||
		status === "partial_expired" ||
		status === "partial_rejected" ||
		status === "rejected"
	);
};

const decisionIdentity = (position: Position): string =>
	typeof position.decision?.id === "string" ? position.decision.id : "";

const observedSymbols = new Set<string>();
const observedSources = new Set<string>();

const observeSymbolsFrom = (rows: Measurement[]): void => {
	const symbols: string[] = [];
	const sources: string[] = [];

	for (const row of rows) {
		if (row.symbol !== "" && !observedSymbols.has(row.symbol)) {
			observedSymbols.add(row.symbol);
			symbols.push(row.symbol);
		}

		if (row.source !== "" && !observedSources.has(row.source)) {
			observedSources.add(row.source);
			sources.push(row.source);
		}
	}

	if (symbols.length > 0) {
		appStore.actions.observeSymbols(symbols);
	}

	if (sources.length > 0) {
		appStore.actions.observeSources(new Set(sources));
	}
};

const currentStrategy = (value: JSONSerializable): StrategyFrame => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		throw new Error("strategy frame requires an object envelope");
	}

	const envelope = value as { decisions?: JSONSerializable; outcome?: unknown; evaluated?: unknown };
	const decisions = (Array.isArray(envelope.decisions)
		? envelope.decisions
		: []) as unknown as Decision[];

	for (const row of decisions) {
		if (typeof row.symbol !== "string" || row.symbol === "") {
			throw new Error("strategy decision requires symbol");
		}
	}

	decisionStore.actions.updateFrame(decisions);

	return {
		outcome: typeof envelope.outcome === "string" ? envelope.outcome : undefined,
		evaluated: typeof envelope.evaluated === "string" ? envelope.evaluated : undefined,
		decisions: latestStrategyDecisions(decisionStore.state.decisions),
	};
};

type Sink = (update: JSONSerializable) => void;

const ingestMeasurements: Sink = (update) => {
	const rows = toMeasurement(update);
	measurementsStore.actions.updateFrame(rows);
	observeSymbolsFrom(rows);
};

const ingestCognition: Sink = (update) => {
	cognitionStore.actions.updateFrame(toCognitive(update));
};

const ingestResonance: Sink = (update) => {
	resonanceStore.actions.updateFrame(toResonance(update));
	paintTerminalResonanceChart(
		latestValues(resonanceStore.state.resonance),
		appStore.state.focusSymbol,
	);
	const focused = latestOf(
		resonanceStore.state.resonance[appStore.state.focusSymbol],
	);

	if (focused === undefined) {
		resonanceFocusStore.setState(() => null);
		return;
	}

	resonanceFocusStore.setState(() => focused);
};

const ingestStrategy: Sink = (update) => {
	strategyStore.setState(() => currentStrategy(update));
};

const ingestPositions: Sink = (update) => {
	for (const position of toPositions(update)) {
		positionsStore.actions.updateFrame([position]);

		if (!positionIsTerminal(position)) {
			continue;
		}

		const identity = decisionIdentity(position);

		if (identity === "") {
			throw new Error("terminal position frame requires decision.id");
		}

		const duplicate = journalStore.state.journal
			.values()
			.some((entry) => decisionIdentity(entry) === identity);

		if (!duplicate) {
			journalStore.actions.updateFrame([position]);
		}
	}
};

/*
The registry maps each wire key to its sink. applyFrame is a single lookup.
*/
const sinks: Record<string, Sink> = {
	tick: (update) => tickStore.setState(() => update as TickFrame),
	equity: (update) => equityStore.setState(() => update as EquityFrame),
	strategy: ingestStrategy,
	activity: (update) => activityStore.setState(() => update as Record<string, string>),
	causal: (update) =>
		causalStore.setState(() => (Array.isArray(update) ? (update as CausalFrame[]) : [update as CausalFrame])),
	graph: (update) => graphStore.setState(() => update as Record<string, unknown>),
	regulator: (update) => regulatorStore.setState(() => update as Record<string, unknown>),
	measurements: ingestMeasurements,
	cognition: ingestCognition,
	resonance: ingestResonance,
	positions: ingestPositions,
	backtest: (update) =>
		appStore.actions.updateBacktest(update as Partial<typeof appStore.state.backtest>),
	hindsight: (update) =>
		appStore.actions.updateBacktest({ hindsight: update as typeof appStore.state.backtest.hindsight }),
};

const applyFrame = (frame: Record<string, unknown>): void => {
	for (const [key, value] of Object.entries(frame)) {
		sinks[key]?.(value as JSONSerializable);
	}
};

export const attach = (worker: Worker): void => {
	let pendingFrames: Record<string, unknown>[] = [];
	let animationFrame: number | null = null;

	const flush = () => {
		animationFrame = null;
		const frames = pendingFrames;
		pendingFrames = [];

		for (const frame of frames) {
			// Each frame runs in its own boundary so one malformed payload
			// cannot abort later frames in the batch or escape the
			// requestAnimationFrame callback.
			try {
				applyFrame(frame);
			} catch (error) {
				console.error("ws-stores: dropped malformed frame", error);
			}
		}

		worker.postMessage({ type: "PAINTED", acknowledgeBackend: false });
	};

	const schedule = () => {
		if (animationFrame !== null) {
			return;
		}

		animationFrame = requestAnimationFrame(flush);
	};

	worker.addEventListener("message", (event: MessageEvent) => {
		if (event.data.type === "DRAW" && event.data.frame !== undefined) {
			pendingFrames.push(event.data.frame as Record<string, unknown>);
			schedule();
			return;
		}

		if (event.data.type !== "DRAW_BATCH" || !Array.isArray(event.data.frames)) {
			return;
		}

		pendingFrames.push(...event.data.frames);
		schedule();
	});
};

/*
useSubscribe mounts a surface, subscribes it to a store, and runs the writer
imperatively on every advance and once for the current value. React never
re-renders on these updates — the writer writes DOM directly. Returns a ref for
the root element.
*/
export const useSubscribe = <T, E extends HTMLElement = HTMLDivElement>(
	store: { state: T; subscribe: (fn: (state: T) => void) => { unsubscribe(): void } },
	writer: (state: T) => void,
	deps: readonly unknown[] = [],
): React.RefObject<E | null> => {
	const ref = useRef<E | null>(null);
	const writerRef = useRef(writer);

	useLayoutEffect(() => {
		writerRef.current = writer;
	});

	useLayoutEffect(() => {
		writerRef.current(store.state);
		const subscription = store.subscribe((state) => writerRef.current(state));

		return () => subscription.unsubscribe();
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [store, ...deps]);

	return ref;
};

/*
useDecisions returns the live strategy decision list reactively so components
that key row membership off it re-render when decision frames arrive. It layers
over useSubscribe, which stays reserved for high-frequency field-text updates.
*/
export const useDecisions = (): Decision[] =>
	useSelector(strategyStore, (state) => state?.decisions ?? []);