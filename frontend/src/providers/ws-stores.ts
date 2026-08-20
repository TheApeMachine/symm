import { createStore } from "@tanstack/store";
import { appStore } from "#/collections/app";
import { latestValues } from "#/collections/circular";
import {
	DECISION_HISTORY_LIMIT,
	decisionStore,
	latestStrategyDecisions,
} from "#/collections/decisions";
import { createKeyedStore } from "#/collections/store";
import { paintTerminalFluidChart } from "#/components/charts/fluid";
import { paintTerminalResonanceChart } from "#/components/charts/resonance";
import type { JSONSerializable, Paint } from "#/components/ui/paint";
import type { Decision } from "#/types/thesis";

type FrameEvent = {
	key: string;
	sequence: number;
	value: JSONSerializable;
};

const RETAINED_FRAME_LIMIT = 1;
export const JOURNAL_ENTRY_LIMIT = DECISION_HISTORY_LIMIT;

const frameEvents = createStore<FrameEvent>({
	key: "",
	sequence: 0,
	value: null,
});
const retainedFrames = createKeyedStore<JSONSerializable>()(
	"frames",
	RETAINED_FRAME_LIMIT,
	(value) => frameKey(value),
);
const measurements = createKeyedStore<JSONSerializable>()(
	"measurements",
	RETAINED_FRAME_LIMIT,
	(value) => measurementIdentity(value),
);
const cognition = createKeyedStore<JSONSerializable>()(
	"cognition",
	RETAINED_FRAME_LIMIT,
	(value) => symbolIdentity(value),
);
const resonance = createKeyedStore<JSONSerializable>()(
	"resonance",
	RETAINED_FRAME_LIMIT,
	(value) => symbolIdentity(value),
);
const positions = createKeyedStore<JSONSerializable>()(
	"positions",
	RETAINED_FRAME_LIMIT,
	(value) => positionIdentity(value),
);
const journal = createKeyedStore<JSONSerializable>()(
	"journal",
	JOURNAL_ENTRY_LIMIT,
);
const observedSymbols = new Set<string>();
const observedSources = new Set<string>();

const frameRows = (value: JSONSerializable): JSONSerializable[] => {
	if (Array.isArray(value)) {
		return value.filter((row): row is JSONSerializable => row !== undefined);
	}

	if (value !== null && typeof value === "object") {
		if (symbolIdentity(value) !== "") {
			return [value];
		}

		return Object.values(value).filter(
			(row): row is JSONSerializable => row !== undefined,
		);
	}

	return [];
};

const symbolIdentity = (value: JSONSerializable): string =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof value.symbol === "string"
		? value.symbol
		: "";

const measurementIdentity = (value: JSONSerializable): string => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return "";
	}

	return typeof value.source === "string" && typeof value.symbol === "string"
		? `${value.source}\u0000${value.symbol}`
		: "";
};

const positionIdentity = (value: JSONSerializable): string => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return "";
	}

	const holding = value.holding;

	return holding !== null &&
		typeof holding === "object" &&
		!Array.isArray(holding) &&
		typeof holding.symbol === "string"
		? holding.symbol
		: "";
};

const decisionIdentity = (value: JSONSerializable): string => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return "";
	}

	const decision = value.decision;

	return decision !== null &&
		typeof decision === "object" &&
		!Array.isArray(decision) &&
		typeof decision.id === "string"
		? decision.id
		: "";
};

const positionIsTerminal = (value: JSONSerializable): boolean =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	(value.status === "canceled" ||
		value.status === "closed" ||
		value.status === "error" ||
		value.status === "expired" ||
		value.status === "fatal" ||
		value.status === "partial_cancelled" ||
		value.status === "partial_expired" ||
		value.status === "partial_rejected" ||
		value.status === "rejected");

const currentCognition = (): Record<string, JSONSerializable | undefined> =>
	Object.fromEntries(
		Object.entries(cognition.state.cognition).flatMap(([symbol, buffer]) => {
			const value = buffer.values().at(-1);

			return value === undefined ? [] : [[symbol, value]];
		}),
	);

const openPositions = (): JSONSerializable[] =>
	Object.values(positions.state.positions).flatMap((buffer) => {
		const value = buffer.values().at(-1);

		return value === undefined || positionIsTerminal(value) ? [] : [value];
	});

const journalEntries = (): JSONSerializable[] => journal.state.journal.values();

const currentStrategy = (value: JSONSerializable): JSONSerializable => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		throw new Error("strategy frame requires an object envelope");
	}

	if (value.decisions === undefined) {
		throw new Error("strategy frame requires decisions");
	}

	const rows = frameRows(value.decisions);

	for (const row of rows) {
		if (symbolIdentity(row) === "") {
			throw new Error("strategy decision requires symbol");
		}
	}

	decisionStore.actions.updateFrame(rows as unknown as Decision[]);

	return {
		...value,
		decisions: latestStrategyDecisions(
			decisionStore.state.decisions,
		) as unknown as JSONSerializable,
	};
};

const frameKey = (value: JSONSerializable): string => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return "";
	}

	return typeof value.key === "string" ? value.key : "";
};

const retainFrame = (key: string, value: JSONSerializable): void => {
	retainedFrames.actions.updateFrame([{ key, value }]);
};

const retainedFrame = (key: string): JSONSerializable | undefined => {
	const row = retainedFrames.state.frames[key]?.values().at(-1);

	if (row === null || typeof row !== "object" || Array.isArray(row)) {
		return undefined;
	}

	return row.value;
};

/*
getLastFrame returns the last complete retained value for a register key.
Sparse measurements are retained by source and symbol; all other wire keys
retain exactly one frame.
*/
export const getLastFrame = (key: string): JSONSerializable | undefined =>
	retainedFrame(key);

export const drawers = {};

/*
registerPainter subscribes a Component directly to the TanStack event store.
No React binding participates in live data delivery.
*/
export const registerPainter = (key: string, paint: Paint): (() => void) => {
	let sequence = frameEvents.state.sequence;
	const subscription = frameEvents.subscribe((event) => {
		if (event.sequence === sequence || event.key !== key) {
			return;
		}

		sequence = event.sequence;
		paint(event.value);
	});

	return () => subscription.unsubscribe();
};

const observeFrame = (key: string, updates: JSONSerializable): void => {
	if (key === "cognition") {
		const symbols = Object.keys(currentCognition()).filter((symbol) => {
			if (observedSymbols.has(symbol)) {
				return false;
			}

			observedSymbols.add(symbol);
			return true;
		});

		if (symbols.length > 0) {
			appStore.actions.observeSymbols(symbols);
		}

		return;
	}

	if (key !== "measurements" && key !== "causal" && key !== "positions") {
		return;
	}

	const symbols: string[] = [];
	const sources: string[] = [];

	for (const row of frameRows(updates)) {
		const symbol = symbolIdentity(row) || positionIdentity(row);

		if (symbol !== "" && !observedSymbols.has(symbol)) {
			observedSymbols.add(symbol);
			symbols.push(symbol);
		}

		if (
			key === "measurements" &&
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			typeof row.source === "string" &&
			!observedSources.has(row.source)
		) {
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

export const paintRegistered = (
	key: string,
	updates: JSONSerializable,
): void => {
	if (key === "measurements") {
		measurements.actions.updateFrame(frameRows(updates));
		retainFrame(key, latestValues(measurements.state.measurements));
	} else {
		retainFrame(key, updates);
	}

	observeFrame(key, updates);

	if (key === "manifold") {
		paintTerminalFluidChart(updates, appStore.state.focusSymbol);
	}

	if (key === "resonance") {
		paintTerminalResonanceChart(updates, appStore.state.focusSymbol);
	}

	frameEvents.setState((previous) => ({
		key,
		sequence: previous.sequence + 1,
		value: updates,
	}));
};

export const RESONANCE_FOCUS = "resonance.focus";
export const JOURNAL = "journal";

/*
attach routes ordered worker batches into bounded TanStack stores. Every wire
frame is applied in arrival order during one browser paint callback; batching
reduces scheduling overhead without coalescing intermediate samples.
*/
export const attach = (worker: Worker) => {
	let pendingFrames: Record<string, unknown>[] = [];
	let animationFrame: number | null = null;

	const applyFrame = (frame: Record<string, unknown>) => {
		for (const [key, value] of Object.entries(frame)) {
			const update = value as JSONSerializable;

			if (key === "activity" || key === "measurements") {
				paintRegistered(key, update);
				continue;
			}

			if (key === "backtest") {
				appStore.actions.updateBacktest(
					update as Partial<typeof appStore.state.backtest>,
				);
				continue;
			}

			if (key === "hindsight") {
				appStore.actions.updateBacktest({
					hindsight: update as typeof appStore.state.backtest.hindsight,
				});
				continue;
			}

			if (key === "cognition") {
				cognition.actions.updateFrame(frameRows(update));
				paintRegistered(key, currentCognition());
				continue;
			}

			if (key === "resonance") {
				resonance.actions.updateFrame(frameRows(update));
				const rows = latestValues(resonance.state.resonance);
				paintRegistered(key, rows);
				const focused = resonance.state.resonance[appStore.state.focusSymbol]
					?.values()
					.at(-1);

				if (focused !== undefined) {
					paintRegistered(RESONANCE_FOCUS, focused);
				}

				continue;
			}

			if (key === "strategy") {
				paintRegistered(key, currentStrategy(update));
				continue;
			}

			if (key !== "positions") {
				paintRegistered(key, update);
				continue;
			}

			for (const position of frameRows(update)) {
				if (positionIdentity(position) === "") {
					throw new Error("position frame requires holding.symbol");
				}

				positions.actions.updateFrame([position]);

				if (!positionIsTerminal(position)) {
					continue;
				}

				const identity = decisionIdentity(position);

				if (identity === "") {
					throw new Error("terminal position frame requires decision.id");
				}

				const duplicate = journal.state.journal
					.values()
					.some((entry) => decisionIdentity(entry) === identity);

				if (!duplicate) {
					journal.actions.updateFrame([position]);
				}
			}

			paintRegistered("positions", openPositions());
			paintRegistered(JOURNAL, journalEntries());
		}
	};

	const flush = () => {
		animationFrame = null;
		const frames = pendingFrames;
		pendingFrames = [];

		try {
			for (const frame of frames) {
				applyFrame(frame);
			}
		} finally {
			worker.postMessage({ type: "PAINTED", acknowledgeBackend: false });
		}
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
