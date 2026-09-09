/*
The React reducer that used to live here has been removed. All node
topology mutations now flow through nodes-actions, which write
directly to researchGraphCollection (TanStack DB). Pure helpers live
in nodes-helpers and nodes-mutations.

This shim re-exports the pure helpers for backward compatibility with
any caller that still imports from "./nodesReducer". Prefer importing
from the focused modules directly.

  - Pure topology helpers:    "./nodes-helpers"
  - Pure mutation helpers:    "./nodes-mutations"
  - Bound write-through API:  "./nodes-actions" + "./useNodeActions"

There is no NodesActionType enum, no NodesAction discriminated union,
no dispatch chain. Topology changes call NodeActions methods.
*/

export {
	buildInitialNodes,
	createNodeActions,
	type NodeActions,
	type NodeActionsEnv,
} from "./nodes-actions";
export {
	emptyConnections,
	getDefaultData,
	normalizeFlumeNode,
	pruneDanglingConnections,
	reconcileNodes,
} from "./nodes-helpers";
export { applyDefaultConnections } from "./nodes-mutations";
