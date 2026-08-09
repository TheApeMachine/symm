import { appStore } from "#/collections/app";
import {
	paintTerminalFluidChart,
	repaintTerminalFluidChart,
} from "#/components/charts/fluid";
import { paintTerminalResonanceChart } from "#/components/charts/resonance";
import type { JSONSerializable, Paint } from "#/components/ui/paint";
import { retainManifoldBinary } from "#/providers/manifold-binary";

const registeredPainters = new Map<string, Set<Paint>>();

/*
lastFrameByKey retains the most recent value painted under each registerKey,
independent of whether anything is currently subscribed to it. A route that
unmounts and remounts a Component loses that instance's own paint history —
routing tears the DOM and the fiber down — so a freshly mounted Component
needs somewhere durable to ask "what was the last frame for this key" and
repaint immediately instead of sitting blank until the next websocket tick.
Bounded to one entry per key: this is a replay cache for "the current state
of the world," not a history, so it cannot grow with time or traffic.
*/
const lastFrameByKey = new Map<string, JSONSerializable>();

/*
getLastFrame lets a freshly mounted Component replay the retained value for
its registerKey immediately on mount, rather than showing nothing until the
next server frame arrives.
*/
export const getLastFrame = (key: string): JSONSerializable | undefined =>
	lastFrameByKey.get(key);

const terminalPositionStatuses = new Set([
	"canceled",
	"closed",
	"error",
	"expired",
	"fatal",
	"rejected",
]);

const frameRows = (value: JSONSerializable): JSONSerializable[] => {
	if (Array.isArray(value)) {
		return value.filter((row): row is JSONSerializable => row !== undefined);
	}

	if (value !== null && typeof value === "object") {
		if (symbolIdentity(value) !== null) {
			return [value];
		}

		return Object.values(value).filter(
			(row): row is JSONSerializable => row !== undefined,
		);
	}

	return [];
};

const positionIdentity = (value: JSONSerializable): string | null => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return null;
	}

	if (typeof value.id === "string" && value.id !== "") {
		return value.id;
	}

	const holding = value.holding;

	if (
		holding !== null &&
		typeof holding === "object" &&
		!Array.isArray(holding) &&
		typeof holding.symbol === "string" &&
		holding.symbol !== ""
	) {
		return holding.symbol;
	}

	return null;
};

const positionIsTerminal = (value: JSONSerializable): boolean =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof value.status === "string" &&
	terminalPositionStatuses.has(value.status);

/*
symbolIdentity names the symbol a per-symbol frame belongs to.
*/
const symbolIdentity = (value: JSONSerializable): string | null =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof value.symbol === "string" &&
	value.symbol !== ""
		? value.symbol
		: null;

/*
cognitionEntries reads the symbol-keyed cognition map off the wire. The
classifier publishes whichever symbols it re-read this tick, so a frame is a
delta over the universe rather than the universe itself.
*/
const cognitionEntries = (
	value: JSONSerializable,
): Array<[string, JSONSerializable]> => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return [];
	}

	return Object.entries(value).filter(
		(entry): entry is [string, JSONSerializable] => entry[1] !== undefined,
	);
};

export const drawers = {};

export const registerPainter = (key: string, paint: Paint): (() => void) => {
	const painters = registeredPainters.get(key) ?? new Set<Paint>();

	painters.add(paint);
	registeredPainters.set(key, painters);

	return () => {
		painters.delete(paint);

		if (painters.size === 0) {
			registeredPainters.delete(key);
		}
	};
};

/*
observeFrame records what a frame reveals about the run's shape: which kernels
have reported, and which symbols the engine has said anything about. Neither is
published as a list of its own, so the only way the palette and the kernel rail
learn them is by watching the frames that name them go past.
*/
const observeFrame = (key: string, updates: JSONSerializable): void => {
	if (key === "cognition") {
		appStore.actions.observeSymbols(
			cognitionEntries(updates).map(([symbol]) => symbol),
		);

		return;
	}

	if (key !== "measurements" && key !== "causal" && key !== "positions") {
		return;
	}

	const rows = frameRows(updates);
	const symbols: string[] = [];
	const sources = new Set<string>();

	for (const row of rows) {
		const symbol = symbolIdentity(row) ?? positionIdentity(row);

		if (symbol !== null) {
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
	lastFrameByKey.set(key, updates);
	observeFrame(key, updates);

	if (key === "manifold") {
		paintTerminalFluidChart(updates, appStore.state.focusSymbol);
	}

	if (key === "resonance") {
		paintTerminalResonanceChart(updates, appStore.state.focusSymbol);
	}

	for (const paint of registeredPainters.get(key) ?? []) {
		paint(updates);
	}
};

/*
RESONANCE_FOCUS is the focused carrier on its own key.

The solver publishes every carrier it settled, because the latent cross-section
plots all of them. A surface reading one carrier wants the inspected symbol, and
picking it out of the batch by position only works once that symbol has settled —
before then index 0 is some other symbol, and the chart would draw its vectors
under the focused symbol's name. Deriving the row here makes "the carrier being
inspected" a stream of its own, so a binding cannot accidentally read a
neighbour.
*/
export const RESONANCE_FOCUS = "resonance.focus";

/*
attach coalesces worker updates to one paint pass per display frame. DRAW values
carry at most one sparse delta per identity from the worker. Positions, cognition
and resonance are retained on the main thread and materialized once when the
display frame begins; other newer values supersede work not yet painted. DRAWN
acknowledges the paint so the worker does not dispatch faster than the display.
*/
export const attach = (worker: Worker) => {
	const pendingUpdates = new Map<string, JSONSerializable>();
	const retainedPositions = new Map<string, JSONSerializable>();
	const retainedCognition = new Map<string, JSONSerializable>();
	const retainedResonance = new Map<string, JSONSerializable>();
	let pendingBinary: ArrayBuffer | null = null;
	let animationFrame: number | null = null;

	const flush = () => {
		animationFrame = null;
		const updates = Array.from(pendingUpdates, ([key, value]) => {
			if (key === "cognition") {
				return [key, Object.fromEntries(retainedCognition)] as const;
			}

			if (key === "resonance") {
				return [key, Array.from(retainedResonance.values())] as const;
			}

			if (key === "positions") {
				return [key, Array.from(retainedPositions.values())] as const;
			}

			return [key, value] as const;
		});
		const binary = pendingBinary;

		pendingUpdates.clear();
		pendingBinary = null;

		for (const [key, value] of updates) {
			paintRegistered(key, value);

			if (key === "resonance") {
				const focused = retainedResonance.get(appStore.state.focusSymbol);

				if (focused !== undefined) {
					paintRegistered(RESONANCE_FOCUS, focused);
				}
			}
		}

		if (binary !== null && retainManifoldBinary(binary)) {
			repaintTerminalFluidChart(appStore.state.focusSymbol);
		}

		worker.postMessage({ type: "DRAWN" });
	};

	const schedule = () => {
		if (animationFrame !== null) {
			return;
		}

		animationFrame = requestAnimationFrame(flush);
	};

	worker.addEventListener("message", (event: MessageEvent) => {
		if (
			event.data.type === "DRAW_BIN" &&
			event.data.buffer instanceof ArrayBuffer
		) {
			pendingBinary = event.data.buffer;
			schedule();
			return;
		}

		if (event.data.type !== "DRAW" || event.data.frame === undefined) {
			return;
		}

		for (const [key, value] of Object.entries(event.data.frame)) {
			const update = value as JSONSerializable;

			if (key === "cognition") {
				for (const [symbol, reading] of cognitionEntries(update)) {
					retainedCognition.set(symbol, reading);
				}

				pendingUpdates.set("cognition", null);
				continue;
			}

			if (key === "resonance") {
				for (const frame of frameRows(update)) {
					const identity = symbolIdentity(frame);

					if (identity !== null) {
						retainedResonance.set(identity, frame);
					}
				}

				pendingUpdates.set("resonance", null);
				continue;
			}

			if (key !== "positions") {
				pendingUpdates.set(key, update);
				continue;
			}

			for (const position of frameRows(update)) {
				const identity = positionIdentity(position);

				if (identity === null) {
					continue;
				}

				if (positionIsTerminal(position)) {
					retainedPositions.delete(identity);
					continue;
				}

				retainedPositions.set(identity, position);
			}

			pendingUpdates.set("positions", null);
		}

		schedule();
	});
};
