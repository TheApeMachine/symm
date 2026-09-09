import { Store, useStore } from "@tanstack/react-store";
import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";

/*
FlumeEditorState holds the ephemeral, view-only state for the Flume
graph editor. Persisted graph topology lives in researchGraphCollection;
this store is only for things that don't survive a page reload or
don't need to round-trip through a backend.

Each editor instance gets its own slice under editorState[editorId] so
two side-by-side editors don't fight each other.
*/
export type FlumeRoutingMode = EdgeRoutingMode;

export type DragOverride = { x: number; y: number };

export type FlumeEditorState = {
	routingMode: FlumeRoutingMode;
	dragOverrideByEditorId: Record<string, Record<string, DragOverride>>;
};

const STORAGE_KEY = "caramba.flume.routingMode";

const readInitialRoutingMode = (): FlumeRoutingMode => {
	if (typeof window === "undefined") return "smooth";

	const stored = window.localStorage.getItem(STORAGE_KEY);

	if (stored === "smooth" || stored === "straight" || stored === "orthogonal") {
		return stored;
	}

	return "smooth";
};

export const flumeEditorStore = new Store<FlumeEditorState>({
	routingMode: readInitialRoutingMode(),
	dragOverrideByEditorId: {},
});

if (typeof window !== "undefined") {
	flumeEditorStore.subscribe(() => {
		const { routingMode } = flumeEditorStore.state;
		window.localStorage.setItem(STORAGE_KEY, routingMode);
	});
}

export const setRoutingMode = (mode: FlumeRoutingMode): void => {
	flumeEditorStore.setState((previous) =>
		previous.routingMode === mode
			? previous
			: { ...previous, routingMode: mode },
	);
};

export const useRoutingMode = (): FlumeRoutingMode =>
	useStore(flumeEditorStore, (state) => state.routingMode);

export const setDragOverride = (
	editorId: string,
	override: Record<string, DragOverride> | null,
): void => {
	flumeEditorStore.setState((previous) => {
		const next = { ...previous.dragOverrideByEditorId };

		if (override === null || Object.keys(override).length === 0) {
			if (!(editorId in next)) {
				return previous;
			}
			delete next[editorId];
		} else {
			next[editorId] = override;
		}

		return { ...previous, dragOverrideByEditorId: next };
	});
};

export const useDragOverride = (
	editorId: string,
): Record<string, DragOverride> | null =>
	useStore(
		flumeEditorStore,
		(state) => state.dragOverrideByEditorId[editorId] ?? null,
	);
