import { eq, useLiveQuery } from "@tanstack/react-db";
import React from "react";
import { researchGraphCollection } from "#/collections/research_graph";
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

/*
useNodesState is the only sanctioned source of truth for Flume graph
topology. It reads from and writes to researchGraphCollection through
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
	isLoading: boolean;
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

	const { data, isLoading } = useLiveQuery(
		(query) =>
			query
				.from({ graph: researchGraphCollection })
				.where(({ graph }) => eq(graph.id, graphId))
				.select(({ graph }) => ({
					id: graph.id,
					nodes: graph.nodes,
				})),
		[graphId],
	);

	const row = data?.[0];
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
			const existing = researchGraphCollection.get(graphId);

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
				researchGraphCollection.insert({
					id: graphId,
					project_id: projectId ?? null,
					schema_version: 1,
					nodes: seeded,
					comments: {},
					viewport: { scale: 1, translate: { x: 0, y: 0 } },
					updated_at: new Date(),
				});
			} catch {
				// Lost the race; useLiveQuery will pick up the winner.
			}
		},
		[graphId, getEnvironment, projectId],
	);

	return {
		nodes,
		actions,
		isLoading,
		hasRow: row !== undefined,
		seed,
	};
};
