import { appStore } from "#/collections/app";
import {
	paintTerminalFluidChart,
	repaintTerminalFluidChart,
} from "#/components/charts/fluid";
import { paintTerminalResonanceChart } from "#/components/charts/resonance";
import type { JSONSerializable, Paint } from "#/components/ui/paint";
import { retainManifoldBinary } from "#/providers/manifold-binary";

const registeredPainters = new Map<string, Set<Paint>>();

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

export const paintRegistered = (
	key: string,
	updates: JSONSerializable,
): void => {
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
attach coalesces worker updates to one paint pass per display frame. DRAW values
are complete per wire key except positions, cognition and resonance, which each
publish one independently owned lot / symbol at a time. Those deltas are retained
by identity so a surface sees the whole universe rather than whichever slice the
engine happened to re-read this tick; other newer values supersede work the
browser has not painted yet.
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
		const updates = Array.from(pendingUpdates.entries());
		const binary = pendingBinary;

		pendingUpdates.clear();
		pendingBinary = null;

		for (const [key, value] of updates) {
			paintRegistered(key, value);
		}

		if (binary === null || !retainManifoldBinary(binary)) {
			return;
		}

		repaintTerminalFluidChart(appStore.state.focusSymbol);
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

				pendingUpdates.set("cognition", Object.fromEntries(retainedCognition));
				continue;
			}

			if (key === "resonance") {
				for (const frame of frameRows(update)) {
					const identity = symbolIdentity(frame);

					if (identity !== null) {
						retainedResonance.set(identity, frame);
					}
				}

				pendingUpdates.set("resonance", Array.from(retainedResonance.values()));
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

			pendingUpdates.set("positions", Array.from(retainedPositions.values()));
		}

		schedule();
	});
};
