import React from "react";
import {
	createNodeActions,
	type NodeActions,
	type NodeActionsEnv,
} from "#/components/flume/nodes-actions";

/*
useNodeActions returns a stable NodeActions object bound to the given
graphId and env accessor. The same object identity is preserved across
renders so consumers reading from NodeActionsContext don't re-render
unnecessarily. getEnv must be a stable callback (typically a ref-backed
reader); the actions read it lazily on each call so they always see
the current nodeTypes/portTypes/context.
*/

export const useNodeActions = (
	graphId: string,
	getEnv: () => NodeActionsEnv,
): NodeActions => {
	const actionsRef = React.useRef<{
		graphId: string;
		actions: NodeActions;
	} | null>(null);

	if (actionsRef.current === null || actionsRef.current.graphId !== graphId) {
		actionsRef.current = {
			graphId,
			actions: createNodeActions(graphId, getEnv),
		};
	}

	return actionsRef.current.actions;
};
