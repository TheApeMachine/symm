import React, { type RefObject } from "react";

import type FlumeCache from "#/components/flume/Cache";
import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";
import type { NodeActions } from "#/components/flume/nodes-actions";
import type {
	FlumeNode,
	NodeMap,
	NodeTypeMap,
	PortTypeMap,
	StageState,
} from "#/components/flume/types";

/** Current edge path style shared by Connections, temporary drag previews, and {@link IoPorts}. */
export const EdgeRoutingContext =
	React.createContext<EdgeRoutingMode>("smooth");

export function useEdgeRouting(): EdgeRoutingMode {
	return React.useContext(EdgeRoutingContext) ?? "smooth";
}

export const NodeTypesContext = React.createContext<NodeTypeMap | null>(null);
export const PortTypesContext = React.createContext<PortTypeMap | null>(null);

/*
NodeActionsContext exposes the bound action functions that write
directly to researchGraphCollection. Replaces the prior NodeDispatchContext
+ useReducer dispatch chain — there is no React reducer in the Flume
pipeline, only TanStack DB and TanStack Store.
*/
export const NodeActionsContext = React.createContext<NodeActions | null>(null);

export const useNodeActionsContext = (): NodeActions => {
	const actions = React.useContext(NodeActionsContext);

	if (!actions) {
		throw new Error(
			"useNodeActionsContext must be called inside a NodeEditor — NodeActionsContext is not set.",
		);
	}

	return actions;
};

export const ConnectionRecalculateContext = React.createContext<
	| ((positionOverrides?: Record<string, { x: number; y: number }>) => void)
	| null
>(null);
export const ContextContext = React.createContext<unknown>(null);
export const StageContext = React.createContext<StageState | null>(null);
export const CacheContext = React.createContext<RefObject<FlumeCache> | null>(
	null,
);
export const RecalculateStageRectContext = React.createContext<
	null | (() => void)
>(null);
export const EditorIdContext = React.createContext<string>("");

/*
GraphIdContext exposes the collection row id the current NodeEditor is
bound to. Subgraph editors derive composite ids from it so they also
persist through researchGraphCollection — no inline state anywhere.
*/
export const GraphIdContext = React.createContext<string>("");

/** Maps node IDs to their full node data for consumers that need the full graph. */
export const NodeMapContext = React.createContext<NodeMap>({});

/** Live drag coordinates for the node currently being moved (single-node drag). */
export const NodeDragOverrideContext = React.createContext<Record<
	string,
	{ x: number; y: number }
> | null>(null);

/*
FlumeGraphWorkerContext exposes the push-only worker handle. Main-thread
callers send state mutations (setGraph, setPortLayout, setNodeLayout,
drag events) and the worker recomputes paths off-thread, calling back
to the DOM via syncConnectionElements + applyPaths internally.
*/
export type FlumeGraphWorkerHandle = {
	beginDrag: (nodeId: string) => void;
	updateDrag: (nodeId: string, x: number, y: number) => void;
	endDrag: (nodeId: string, x: number, y: number) => void;
	recalculate: (
		nodes: NodeMap,
		positionOverrides?: Record<string, { x: number; y: number }>,
	) => void;
	setGraph: (nodes: NodeMap) => void;
	setPortLayout: (
		nodeId: string,
		portName: string,
		transputType: "input" | "output",
		offsetX: number,
		offsetY: number,
	) => void;
	setNodeLayout: (nodeId: string, width: number, height: number) => void;
	scheduleRender: () => void;
};

export const FlumeGraphWorkerContext =
	React.createContext<FlumeGraphWorkerHandle | null>(null);

/** @deprecated Use {@link FlumeGraphWorkerContext}. */
export const RecalculateConnectionsWorkerContext = FlumeGraphWorkerContext;

/*
SubGraphContext is set by a block Node when it renders an inline NodeEditor.
It gives the nested editor a callback to write its NodeMap back into the
parent node's subGraph field, keeping the outer graph in sync.
*/
export const SubGraphContext = React.createContext<
	((subGraph: NodeMap) => void) | null
>(null);

// Re-export FlumeNode so callers that import from context don't need a second import.
export type { FlumeNode, NodeMap };
