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
		appStore.actions.observeSymbols(Object.keys(currentCognition()));
		return;
	}

	if (key !== "measurements" && key !== "causal" && key !== "positions") {
		return;
	}

	const symbols: string[] = [];
	const sources = new Set<string>();

	for (const row of frameRows(updates)) {
		const symbol = symbolIdentity(row) || positionIdentity(row);

		if (symbol !== "") {
			symbols.push(symbol);
		}

		if (
			key === "measurements" &&
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			typeof row.source === "string"
		) {
			sources.add(row.source);
		}
	}

	appStore.actions.observeSymbols(symbols);
	appStore.actions.observeSources(sources);
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
attach routes DRAW frames into bounded TanStack stores. Sparse measurements
paint immediately; complete aggregate domains coalesce to one animation frame.
*/
export const attach = (worker: Worker) => {
	const pendingUpdates = new Map<string, JSONSerializable>();
	let animationFrame: number | null = null;

	const flush = () => {
		animationFrame = null;
		const updates = [...pendingUpdates];
		pendingUpdates.clear();

		for (const [key, value] of updates) {
			paintRegistered(key, value);
		}
	};

	const schedule = () => {
		if (animationFrame !== null) {
			return;
		}

		animationFrame = requestAnimationFrame(flush);
	};

	worker.addEventListener("message", (event: MessageEvent) => {
		if (event.data.type !== "DRAW" || event.data.frame === undefined) {
			return;
		}

		for (const [key, value] of Object.entries(event.data.frame)) {
			const update = value as JSONSerializable;

			if (key === "activity" || key === "measurements") {
				paintRegistered(key, update);
				continue;
			}

			if (key === "cognition") {
				cognition.actions.updateFrame(frameRows(update));
				pendingUpdates.set(key, currentCognition());
				continue;
			}

			if (key === "resonance") {
				resonance.actions.updateFrame(frameRows(update));
				const rows = latestValues(resonance.state.resonance);
				pendingUpdates.set(key, rows);
				const focused = resonance.state.resonance[
					appStore.state.focusSymbol
				]?.values().at(-1);

				if (focused !== undefined) {
					pendingUpdates.set(RESONANCE_FOCUS, focused);
				}

				continue;
			}

			if (key === "strategy") {
				pendingUpdates.set(key, currentStrategy(update));
				continue;
			}

			if (key !== "positions") {
				pendingUpdates.set(key, update);
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

				const duplicate = journal.state.journal.values().some(
					(entry) => decisionIdentity(entry) === identity,
				);

				if (!duplicate) {
					journal.actions.updateFrame([position]);
				}
			}

			pendingUpdates.set("positions", openPositions());
			pendingUpdates.set(JOURNAL, journalEntries());
		}

		if (pendingUpdates.size > 0) {
			schedule();
		}
	});
};
