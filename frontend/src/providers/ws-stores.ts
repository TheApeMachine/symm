import { appStore } from "#/collections/app";
import { repaintTerminalFluidChart } from "#/components/charts/fluid";
import type { JSONSerializable, Paint } from "#/components/ui/paint";
import { retainManifoldBinary } from "#/providers/manifold-binary";

const registeredPainters = new Map<string, Set<Paint>>();

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
	for (const paint of registeredPainters.get(key) ?? []) {
		paint(updates);
	}
};

/*
attach coalesces worker updates to one paint pass per display frame. DRAW values
are complete per wire key, so a newer value supersedes work the browser has not
painted yet without merging unrelated backend categories.
*/
export const attach = (worker: Worker) => {
	const pendingUpdates = new Map<string, JSONSerializable>();
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
			pendingUpdates.set(key, value as JSONSerializable);
		}

		schedule();
	});
};
