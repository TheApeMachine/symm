import { useCallback, useSyncExternalStore } from "react";
import {
	type PipelineGraphRowType,
	pipelineGraphCollection,
} from "#/collections/pipeline_graph";

/*
usePipelineGraphRow reads one drawn pipeline and re-renders when it changes.

The collection ships a live-query DSL, and this deliberately does not use it.
Compiling a query over this row fails inside the library the moment the row is
updated — "Query contributors with the same row key are not congruent", raised
by the compiler's reduce step — which left an editor that persisted every edit
and displayed none of them until the page was reloaded. The reads here want one
row by its key, which is the one thing a collection can answer without a query
compiler at all.

Reading through useSyncExternalStore rather than mirroring the row into state
keeps the collection the only copy: there is no second source to fall out of
step with it, and a render always shows what was actually persisted.
*/
export const usePipelineGraphRow = (
	graphId: string,
): PipelineGraphRowType | undefined => {
	const subscribe = useCallback((onChange: () => void) => {
		const subscription = pipelineGraphCollection.subscribeChanges(onChange);

		return () => subscription.unsubscribe();
	}, []);

	/*
		The collection returns the same object identity until the row is
		replaced, so this is a valid getSnapshot: React re-renders exactly when
		the stored row is a different object.
	*/
	const read = useCallback(
		() => pipelineGraphCollection.get(graphId),
		[graphId],
	);

	// Server rendering has no collection to read; the editor is client-only.
	return useSyncExternalStore(subscribe, read, () => undefined);
};
