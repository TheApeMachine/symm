import { eq, useLiveQuery } from "@tanstack/react-db";
import React from "react";
import { researchGraphCollection } from "#/collections/research_graph";
import commentsReducer, {
	type CommentAction,
} from "#/components/flume/commentsReducer";
import type {
	StageActionSetter,
	StageState,
} from "#/components/flume/stageReducer";
import stageReducer from "#/components/flume/stageReducer";
import type { FlumeCommentMap, StageTranslate } from "#/components/flume/types";

const DEFAULT_VIEWPORT: StageState = {
	scale: 1,
	translate: { x: 0, y: 0 },
};

/*
useCommentsState mirrors the shape of useNodesState but for the comment
map on the row. Single collection-backed path — no local useReducer.
*/
export const useCommentsState = (
	graphId: string,
): {
	comments: FlumeCommentMap;
	dispatch: React.Dispatch<CommentAction>;
} => {
	const { data } = useLiveQuery(
		(query) =>
			query
				.from({ graph: researchGraphCollection })
				.where(({ graph }) => eq(graph.id, graphId))
				.select(({ graph }) => ({
					id: graph.id,
					comments: graph.comments,
				})),
		[graphId],
	);

	const comments = (data?.[0]?.comments as FlumeCommentMap | undefined) ?? {};

	const dispatch = React.useCallback<React.Dispatch<CommentAction>>(
		(action) => {
			researchGraphCollection.update(graphId, (draft) => {
				const current = (draft.comments as FlumeCommentMap | undefined) ?? {};
				const next = commentsReducer(current, action);

				if (next === current) {
					return;
				}

				draft.comments = next;
				draft.updated_at = new Date();
			});
		},
		[graphId],
	);

	return { comments, dispatch };
};

/*
useViewportState owns the persisted scale/translate for the editor's
stage. Continuous pan/zoom writes pass through a rAF-coalesced commit
so the collection isn't flooded with updates per pixel of motion.
*/
export const useViewportState = (
	graphId: string,
): {
	viewport: StageState;
	dispatch: React.Dispatch<StageActionSetter>;
} => {
	const { data } = useLiveQuery(
		(query) =>
			query
				.from({ graph: researchGraphCollection })
				.where(({ graph }) => eq(graph.id, graphId))
				.select(({ graph }) => ({
					id: graph.id,
					viewport: graph.viewport,
				})),
		[graphId],
	);

	const stored = data?.[0]?.viewport as
		| { scale?: number; translate?: StageTranslate }
		| undefined;

	const viewport = React.useMemo<StageState>(
		() => ({
			scale: stored?.scale ?? DEFAULT_VIEWPORT.scale,
			translate: stored?.translate ?? DEFAULT_VIEWPORT.translate,
		}),
		[stored?.scale, stored?.translate],
	);

	const pendingRef = React.useRef<StageState | null>(null);
	const frameRef = React.useRef<number | null>(null);

	const flush = React.useCallback(() => {
		frameRef.current = null;
		const pending = pendingRef.current;
		pendingRef.current = null;

		if (!pending) return;

		researchGraphCollection.update(graphId, (draft) => {
			draft.viewport = pending;
			draft.updated_at = new Date();
		});
	}, [graphId]);

	const dispatch = React.useCallback<React.Dispatch<StageActionSetter>>(
		(incoming) => {
			const previous = pendingRef.current ?? viewport;
			const action =
				typeof incoming === "function" ? incoming(previous) : incoming;
			const next = stageReducer(previous, action);

			if (next === previous) {
				return;
			}

			pendingRef.current = next;

			if (frameRef.current !== null) return;
			frameRef.current = requestAnimationFrame(flush);
		},
		[flush, viewport],
	);

	React.useEffect(() => {
		return () => {
			if (frameRef.current !== null) {
				cancelAnimationFrame(frameRef.current);
				frameRef.current = null;
			}

			// Drop any uncommitted state; on remount we hydrate fresh.
			pendingRef.current = null;
		};
	}, []);

	return { viewport, dispatch };
};
