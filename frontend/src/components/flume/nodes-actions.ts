import type { RefObject } from "react";
import { researchGraphCollection } from "#/collections/research_graph";
import type FlumeCache from "#/components/flume/Cache";
import { deleteConnection } from "#/components/flume/connectionCalculator";
import {
	getDefaultData,
	pruneDanglingConnections,
	reconcileNodes,
} from "#/components/flume/nodes-helpers";
import {
	addConnection as addConnectionPure,
	type ProposedConnection,
	remapConnectionNodeIds,
	removeConnection as removeConnectionPure,
	removeNode as removeNodePure,
} from "#/components/flume/nodes-mutations";
import type {
	CircularBehavior,
	FlumeNode,
	InputData,
	NodeMap,
	NodeTypeMap,
	PortTypeMap,
	TransputType,
	ValueSetter,
} from "#/components/flume/types";
import {
	checkForCircularNodes,
	createFlumeId,
} from "#/components/flume/utilities";

/*
nodes-actions replaces the old NodesAction enum + nodesReducer + dispatch
chain. Each action is a module-level function bound at hook time to a
graphId and an env accessor. The action writes directly to
researchGraphCollection.update so TanStack DB is the only mover of
state. There is no intermediate React reducer.

createNodeActions returns a stable object of action callbacks. Consumers
read the object through NodeActionsContext; the object identity is held
inside a ref in useNodeActions so re-renders never produce a new
actions instance.
*/

export type { ProposedConnection } from "#/components/flume/nodes-mutations";

export interface NodeActionsEnv {
	nodeTypes: NodeTypeMap;
	portTypes: PortTypeMap;
	context: unknown;
	cache?: RefObject<FlumeCache>;
	circularBehavior?: CircularBehavior;
}

export interface NodeActions {
	addConnection: (
		input: ProposedConnection,
		output: ProposedConnection,
	) => void;
	removeConnection: (
		input: ProposedConnection,
		output: ProposedConnection,
	) => void;
	destroyTransput: (
		transput: ProposedConnection,
		transputType: TransputType,
	) => void;
	addNode: (params: {
		x: number;
		y: number;
		nodeType: string;
		id?: string;
		defaultNode?: boolean;
	}) => string | undefined;
	removeNode: (nodeId: string) => void;
	hydrateDefaultNodes: () => void;
	setPortData: (params: {
		nodeId: string;
		portName: string;
		controlName: string;
		data: unknown;
		setValue?: ValueSetter;
	}) => void;
	setNodeCoordinates: (params: {
		nodeId: string;
		x: number;
		y: number;
	}) => void;
	setNodeDimensions: (params: {
		nodeId: string;
		width: number;
		height: number;
	}) => void;
	setNodeSubGraph: (params: { nodeId: string; subGraph: NodeMap }) => void;
	applyNodeCoordinates: (
		updates: ReadonlyArray<{ nodeId: string; x: number; y: number }>,
	) => void;
	reconcileNodeTypes: () => void;
}

type Mutator = (current: NodeMap, env: NodeActionsEnv) => NodeMap | undefined;

const runMutation = (
	graphId: string,
	getEnv: () => NodeActionsEnv,
	mutator: Mutator,
): void => {
	if (!researchGraphCollection.get(graphId)) {
		return;
	}

	const env = getEnv();

	researchGraphCollection.update(graphId, (draft) => {
		const raw = (draft.nodes as NodeMap | undefined) ?? {};
		const current = reconcileNodes(
			raw,
			env.nodeTypes,
			env.portTypes,
			env.context,
		);
		const next = mutator(current, env);

		if (next === undefined || next === current) {
			return;
		}

		draft.nodes = next;
		draft.updated_at = new Date();
	});
};

const buildNewNode = (
	params: {
		x: number;
		y: number;
		nodeType: string;
		id?: string;
		defaultNode?: boolean;
	},
	env: NodeActionsEnv,
): FlumeNode | undefined => {
	const { x, y, nodeType, id, defaultNode } = params;
	const nodeTypeDef = env.nodeTypes[nodeType];

	if (!nodeTypeDef) {
		return undefined;
	}

	const newNodeId = id ?? createFlumeId();
	const newNode: FlumeNode = {
		id: newNodeId,
		x,
		y,
		type: nodeType,
		width: nodeTypeDef.initialWidth ?? 200,
		connections: { inputs: {}, outputs: {} },
		inputData: {},
	};
	newNode.inputData = getDefaultData({
		node: newNode,
		nodeType: nodeTypeDef,
		portTypes: env.portTypes,
		context: env.context,
	});

	if (defaultNode) {
		newNode.defaultNode = true;
	}

	if (nodeTypeDef.root) {
		newNode.root = true;
	}

	if (nodeTypeDef.defaultSubGraph) {
		newNode.subGraph = nodeTypeDef.defaultSubGraph;
	}

	return newNode;
};

const dropConnectionPair = (
	current: NodeMap,
	input: ProposedConnection,
	output: ProposedConnection,
	cache: NodeActionsEnv["cache"],
): NodeMap => {
	const id = `${output.nodeId}|${output.portName}|${input.nodeId}|${input.portName}`;

	if (cache?.current?.connections) {
		delete cache.current.connections[id];
	}

	deleteConnection({ id });
	return removeConnectionPure(current, input, output);
};

/*
patchNode applies a shallow patch to a single node, returning undefined
when the node is missing or when every patched field already matches.
Centralizes the "node exists + something changed" guard that every
single-node setter would otherwise repeat.
*/
const patchNode = (
	current: NodeMap,
	nodeId: string,
	patch: Partial<FlumeNode>,
): NodeMap | undefined => {
	const node = current[nodeId];

	if (!node) {
		return undefined;
	}

	let changed = false;

	for (const key of Object.keys(patch) as (keyof FlumeNode)[]) {
		if (node[key] !== patch[key]) {
			changed = true;
			break;
		}
	}

	if (!changed) {
		return undefined;
	}

	return { ...current, [nodeId]: { ...node, ...patch } };
};

export const createNodeActions = (
	graphId: string,
	getEnv: () => NodeActionsEnv,
): NodeActions => {
	const mutate = (mutator: Mutator) => runMutation(graphId, getEnv, mutator);

	return {
		addConnection: (input, output) =>
			mutate((current, env) => {
				const inputNode = current[input.nodeId];

				if (!inputNode?.connections) {
					return undefined;
				}

				const existingInputLinks =
					inputNode.connections.inputs[input.portName] ?? [];

				if (existingInputLinks.length > 0) {
					return undefined;
				}

				const allowCircular =
					env.circularBehavior === "warn" || env.circularBehavior === "allow";
				const next = addConnectionPure(current, input, output);
				const isCircular = checkForCircularNodes(next, output.nodeId);

				if (isCircular && !allowCircular) {
					return undefined;
				}

				return next;
			}),

		removeConnection: (input, output) =>
			mutate((current, env) =>
				dropConnectionPair(current, input, output, env.cache),
			),

		destroyTransput: (transput, transputType) =>
			mutate((current, env) => {
				const portId = transput.nodeId + transput.portName + transputType;

				if (env.cache?.current?.ports) {
					delete env.cache.current.ports[portId];
				}

				const cnxType = transputType === "input" ? "inputs" : "outputs";
				const node = current[transput.nodeId];
				const connections = node?.connections?.[cnxType]?.[transput.portName];

				if (!connections?.length) {
					return undefined;
				}

				return connections.reduce<NodeMap>((acc, cnx) => {
					const [input, output] =
						transputType === "input" ? [transput, cnx] : [cnx, transput];
					return dropConnectionPair(acc, input, output, env.cache);
				}, current);
			}),

		addNode: (params) => {
			let newId: string | undefined;

			mutate((current, env) => {
				const newNode = buildNewNode(params, env);

				if (!newNode) {
					return undefined;
				}

				newId = newNode.id;
				return { ...current, [newNode.id]: newNode };
			});

			return newId;
		},

		removeNode: (nodeId) =>
			mutate((current) => removeNodePure(current, nodeId)),

		hydrateDefaultNodes: () =>
			mutate((current) => {
				const idMap = new Map<string, string>();
				let next: NodeMap = { ...current };

				for (const key of Object.keys(next)) {
					if (!next[key].defaultNode) {
						continue;
					}

					const newNodeId = createFlumeId();
					idMap.set(key, newNodeId);
					const { id: _oldId, defaultNode: _oldDefault, ...node } = next[key];
					next[newNodeId] = { ...node, id: newNodeId };
					delete next[key];
				}

				if (idMap.size === 0) {
					return undefined;
				}

				next = Object.fromEntries(
					Object.entries(next).map(([nodeId, node]) => [
						nodeId,
						{
							...node,
							connections: remapConnectionNodeIds(node.connections, idMap),
						},
					]),
				);

				return pruneDanglingConnections(next);
			}),

		setPortData: ({ nodeId, portName, controlName, data, setValue }) =>
			mutate((current) => {
				const node = current[nodeId];

				if (!node) {
					return undefined;
				}

				let newData: Record<string, unknown> = {
					...node.inputData,
					[portName]: { ...node.inputData[portName], [controlName]: data },
				};

				if (setValue) {
					newData = setValue(newData, node.inputData) as Record<
						string,
						unknown
					>;
				}

				return {
					...current,
					[nodeId]: { ...node, inputData: newData as InputData },
				};
			}),

		setNodeCoordinates: ({ nodeId, x, y }) =>
			mutate((current) => patchNode(current, nodeId, { x, y })),

		applyNodeCoordinates: (updates) =>
			mutate((current) => {
				let changed = false;
				const next: NodeMap = { ...current };

				for (const { nodeId, x, y } of updates) {
					const node = next[nodeId];

					if (!node || (node.x === x && node.y === y)) {
						continue;
					}

					next[nodeId] = { ...node, x, y };
					changed = true;
				}

				return changed ? next : undefined;
			}),

		setNodeDimensions: ({ nodeId, width, height }) =>
			mutate((current) => patchNode(current, nodeId, { width, height })),

		setNodeSubGraph: ({ nodeId, subGraph }) =>
			mutate((current) => patchNode(current, nodeId, { subGraph })),

		reconcileNodeTypes: () =>
			mutate((current, env) =>
				reconcileNodes(current, env.nodeTypes, env.portTypes, env.context),
			),
	};
};

export { buildInitialNodes } from "./nodes-seed";
