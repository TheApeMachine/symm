import type { JSONSerializable, Paint } from "#/components/ui/paint";

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

export const paintRegistered = (key: string, updates: JSONSerializable): void => {
	for (const paint of registeredPainters.get(key) ?? []) {
		paint(updates);
	}
};

/*
attach dispatches DRAW frame entries to registered painters.
*/
export const attach = (
	worker: Worker,
) => {
	worker.addEventListener("message", (event: MessageEvent) => {
		if (event.data.type !== "DRAW" || event.data.frame === undefined) {
			return;
		}

		for (const [key, value] of Object.entries(event.data.frame)) {
			paintRegistered(key, value as JSONSerializable);
		}
	});
};
