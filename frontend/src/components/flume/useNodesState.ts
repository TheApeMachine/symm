import React from "react";
import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import { usePipelineGraphRow } from "#/collections/pipeline_graph_row";
import {
	buildInitialNodes,
	type NodeActions,
	type NodeActionsEnv,
} from "#/components/flume/nodes-actions";
import { reconcileNodes } from "#/components/flume/nodes-helpers";
import type {
	DefaultConnection,
	DefaultNode,
	NodeMap,
	NodeTypeMap,
	PortTypeMap,
} from "#/components/flume/types";
import { useNodeActions } from "#/components/flume/useNodeActions";
import { toastManager } from "#/components/ui/toast";

/*
useNodesState is the only sanctioned source of truth for Flume graph
topology. It reads from and writes to pipelineGraphCollection through
the NodeActions API — there is no React reducer in the loop. Subgraph
editors get composite graphIds (e.g. "parent:nodeId") so they persist
through the same collection.
*/

export type UseNodesStateOptions = {
	graphId: string;
	projectId?: string | null;
	nodeTypes: NodeTypeMap;
	portTypes: PortTypeMap;
	context: unknown;
	getEnvironment: () => NodeActionsEnv;
};

export type UseNodesStateResult = {
	nodes: NodeMap;
	actions: NodeActions;
	hasRow: boolean;
	/**
	 * Inserts an initial topology built from defaultNodes/defaultConnections.
	 * Idempotent: returns silently if a row already exists.
	 */
	seed: (params: {
		defaultNodes?: DefaultNode[];
		defaultConnections?: DefaultConnection[];
	}) => void;
};

export const useNodesState = (
	options: UseNodesStateOptions,
): UseNodesStateResult => {
	const { graphId, projectId, nodeTypes, portTypes, context, getEnvironment } =
		options;

	const row = usePipelineGraphRow(graphId);
	const rawNodes = (row?.nodes as NodeMap | undefined) ?? {};

	// Normalize on every read: persisted rows can drift from the current
	// node/port type registry (operations added, removed, signatures
	// changed). reconcileNodes drops unknown types, fills missing port
	// slots, and refreshes default inputData. This keeps the worker and
	// renderer fed with a valid FlumeNode shape regardless of what was
	// persisted, without forcing a write back to the collection unless
	// the user actually edits.
	const nodes = React.useMemo(
		() => reconcileNodes(rawNodes, nodeTypes, portTypes, context),
		[rawNodes, nodeTypes, portTypes, context],
	);

	const actions = useNodeActions(graphId, getEnvironment);

	const seed = React.useCallback<UseNodesStateResult["seed"]>(
		({ defaultNodes, defaultConnections }) => {
			const existing = pipelineGraphCollection.get(graphId);

			if (existing) {
				return;
			}

			const seeded = buildInitialNodes({
				initialNodes: {},
				defaultNodes: defaultNodes ?? [],
				defaultConnections: defaultConnections ?? [],
				env: getEnvironment(),
			});

			try {
				pipelineGraphCollection.insert({
					id: graphId,
					project_id: projectId ?? null,
					schema_version: 1,
					nodes: seeded,
					comments: {},
					viewport: { scale: 1, translate: { x: 0, y: 0 } },
					updated_at: new Date(),
				});
			} catch (cause) {
				/*
					A concurrent editor inserting the same row first is the
					expected loss here and needs no report — the live query
					picks up the winner. Anything else is a real failure of
					persistence, and swallowing it leaves an editor that
					accepts every edit and keeps none.
				*/
				if (!pipelineGraphCollection.get(graphId)) {
					toastManager.add({
						title: "Pipeline not saved",
						description: cause instanceof Error ? cause.message : String(cause),
						type: "error",
						timeout: 10_000,
					});
				}
			}
		},
		[graphId, getEnvironment, projectId],
	);

	return {
		nodes,
		actions,
		hasRow: row !== undefined,
		seed,
	};
};
