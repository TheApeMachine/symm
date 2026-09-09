import { deleteConnectionsByNodeId } from "#/components/flume/connectionCalculator";
import type {
	ConnectionMap,
	Connections,
	ControlData,
	FlumeNode,
	InputData,
	NodeMap,
	NodeType,
	NodeTypeMap,
	PortTypeMap,
} from "#/components/flume/types";

/*
nodes-helpers contains the pure topology helpers shared by the reducer
and the collection-backed hook. These functions never read from the
DOM (except deleteConnectionsByNodeId, which is a guarded clean-up of
stale SVG when an unknown node type is encountered) and have no
dependence on the reducer's action handlers — they're safe to call
from any layer.
*/

export const emptyConnections = (): Connections => ({
	inputs: {},
	outputs: {},
});

/*
normalizeFlumeNode fills missing graph fields on persisted or partial
nodes. Local-storage rows are schema-loose and may omit connections or
inputData.
*/
export const normalizeFlumeNode = (node: FlumeNode): FlumeNode => ({
	...node,
	inputData: node.inputData ?? {},
	connections: {
		inputs: node.connections?.inputs ?? {},
		outputs: node.connections?.outputs ?? {},
	},
});

/*
pruneDanglingConnections removes input/output links whose endpoint
node no longer exists. Persisted graphs often keep stale ids after
default-node hydration or after a node is removed elsewhere.
*/
export const pruneDanglingConnections = (nodes: NodeMap): NodeMap => {
	const nodeIds = new Set(Object.keys(nodes));
	let changed = false;
	const next: NodeMap = {};

	for (const [nodeId, node] of Object.entries(nodes)) {
		const connections = node.connections ?? emptyConnections();
		const inputs: ConnectionMap = {};

		for (const [portName, links] of Object.entries(connections.inputs)) {
			const filtered = links.filter((link) => nodeIds.has(link.nodeId));

			if (filtered.length !== links.length) {
				changed = true;
			}

			if (filtered.length > 0) {
				inputs[portName] = filtered;
			}
		}

		const outputs: ConnectionMap = {};

		for (const [portName, links] of Object.entries(connections.outputs)) {
			const filtered = links.filter((link) => nodeIds.has(link.nodeId));

			if (filtered.length !== links.length) {
				changed = true;
			}

			if (filtered.length > 0) {
				outputs[portName] = filtered;
			}
		}

		next[nodeId] = {
			...node,
			connections: { inputs, outputs },
		};
	}

	return changed ? next : nodes;
};

/*
getDefaultData walks a node type's input definitions and produces the
matching default inputData object. Used during initial seed and on
read-side reconciliation to keep persisted rows aligned with the
current operation registry.
*/
export const getDefaultData = ({
	node,
	nodeType,
	portTypes,
	context,
}: {
	node: FlumeNode;
	nodeType: NodeType;
	portTypes: PortTypeMap;
	context: unknown;
}): InputData => {
	if (!nodeType) {
		return {};
	}

	const nodeConnections = node.connections ?? emptyConnections();
	const nodeInputData = node.inputData ?? {};

	const inputs = Array.isArray(nodeType.inputs)
		? nodeType.inputs
		: (nodeType.inputs?.(nodeInputData, nodeConnections, context) ?? []);

	return inputs.reduce<InputData>((obj, input) => {
		const inputType = portTypes[input.type];
		obj[input.name || inputType.name] = (
			input.controls ||
			inputType.controls ||
			[]
		).reduce<InputData>((obj2, control) => {
			obj2[control.name] = control.defaultValue as ControlData;
			return obj2;
		}, {});
		return obj;
	}, {});
};

/*
reconcileNodes drops unknown node types, fills missing port slots,
refreshes default inputData against the current registry, and prunes
dangling connections. Run this whenever a persisted row is loaded so
the rest of the editor never sees a malformed shape.
*/
export const reconcileNodes = (
	initialNodes: NodeMap,
	nodeTypes: NodeTypeMap,
	portTypes: PortTypeMap,
	context: unknown,
): NodeMap => {
	const knownNodes: NodeMap = {};

	for (const [nodeId, node] of Object.entries(initialNodes)) {
		if (node?.type && nodeTypes[node.type]) {
			knownNodes[nodeId] = normalizeFlumeNode(node);
			continue;
		}

		if (typeof document !== "undefined") {
			deleteConnectionsByNodeId(nodeId);
		}
	}

	let reconciledNodes = Object.values(knownNodes).reduce<NodeMap>(
		(nodesObj, node) => {
			const nodeType = nodeTypes[node.type];

			if (!nodeType) {
				return nodesObj;
			}

			const defaultInputData = getDefaultData({
				node,
				nodeType,
				portTypes,
				context,
			});
			const currentInputData = Object.entries(node.inputData).reduce<InputData>(
				(dataObj, [key, data]) => {
					if (defaultInputData[key] !== undefined) {
						dataObj[key] = data;
					}
					return dataObj;
				},
				{},
			);

			nodesObj[node.id] = {
				...node,
				inputData: { ...defaultInputData, ...currentInputData },
			};

			return nodesObj;
		},
		{},
	);

	reconciledNodes = Object.values(reconciledNodes).reduce<NodeMap>(
		(nodesObj, node) => {
			const nodeType = nodeTypes[node.type];

			if (!nodeType) {
				return nodesObj;
			}

			const newNode = { ...node };

			if (nodeType.root !== node.root) {
				if (nodeType.root && !node.root) {
					newNode.root = nodeType.root;
				} else if (!nodeType.root && node.root) {
					delete newNode.root;
				}
			}

			nodesObj[node.id] = newNode;
			return nodesObj;
		},
		{},
	);

	return pruneDanglingConnections(reconciledNodes);
};
