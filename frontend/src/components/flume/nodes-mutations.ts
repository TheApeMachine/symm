import { deleteConnectionsByNodeId } from "#/components/flume/connectionCalculator";
import type {
	Connection,
	ConnectionMap,
	Connections,
	DefaultConnection,
	FlumeNode,
	NodeMap,
} from "#/components/flume/types";

/*
nodes-mutations holds the pure NodeMap → NodeMap transformations used by
the reducer. Each function takes the current map plus a small payload
and returns a new map with the requested change applied. They have no
context of action types or environment — keeps the reducer free of
inline data-shuffling so it can read as a routing table over actions.
*/

export type ProposedConnection = { nodeId: string; portName: string };

export const addConnection = (
	nodes: NodeMap,
	input: ProposedConnection,
	output: ProposedConnection,
): NodeMap => {
	const inputNode = nodes[input.nodeId];
	const outputNode = nodes[output.nodeId];

	if (!inputNode?.connections || !outputNode?.connections) {
		return nodes;
	}

	return {
		...nodes,
		[input.nodeId]: {
			...inputNode,
			connections: {
				...inputNode.connections,
				inputs: {
					...inputNode.connections.inputs,
					[input.portName]: [
						...(inputNode.connections.inputs[input.portName] || []),
						{ nodeId: output.nodeId, portName: output.portName },
					],
				},
			},
		},
		[output.nodeId]: {
			...outputNode,
			connections: {
				...outputNode.connections,
				outputs: {
					...outputNode.connections.outputs,
					[output.portName]: [
						...(outputNode.connections.outputs[output.portName] || []),
						{ nodeId: input.nodeId, portName: input.portName },
					],
				},
			},
		},
	};
};

export const removeConnection = (
	nodes: NodeMap,
	input: ProposedConnection,
	output: ProposedConnection,
): NodeMap => {
	const inputNode = nodes[input.nodeId];
	const {
		[input.portName]: _removedInputPort,
		...newInputNodeConnectionsInputs
	} = inputNode.connections.inputs;
	const newInputNode = {
		...inputNode,
		connections: {
			...inputNode.connections,
			inputs: newInputNodeConnectionsInputs,
		},
	};

	const outputNode = nodes[output.nodeId];
	const filteredOutputNodes = outputNode.connections.outputs[
		output.portName
	].filter((cnx) =>
		cnx.nodeId === input.nodeId ? cnx.portName !== input.portName : true,
	);
	const newOutputNode = {
		...outputNode,
		connections: {
			...outputNode.connections,
			outputs: {
				...outputNode.connections.outputs,
				[output.portName]: filteredOutputNodes,
			},
		},
	};

	return {
		...nodes,
		[input.nodeId]: newInputNode,
		[output.nodeId]: newOutputNode,
	};
};

const getFilteredTransputs = (
	transputs: ConnectionMap,
	nodeId: string,
): ConnectionMap =>
	Object.entries(transputs).reduce<ConnectionMap>(
		(obj, [portName, transput]) => {
			const newTransputs = transput.filter((t) => t.nodeId !== nodeId);

			if (newTransputs.length) {
				obj[portName] = newTransputs;
			}

			return obj;
		},
		{},
	);

const removeConnections = (
	connections: Connections,
	nodeId: string,
): Connections => ({
	inputs: getFilteredTransputs(connections.inputs, nodeId),
	outputs: getFilteredTransputs(connections.outputs, nodeId),
});

export const remapConnectionNodeIds = (
	connections: Connections,
	idMap: Map<string, string>,
): Connections => {
	const remapLinks = (links: Connection[]) =>
		links.map((link) => ({
			...link,
			nodeId: idMap.get(link.nodeId) ?? link.nodeId,
		}));

	return {
		inputs: Object.fromEntries(
			Object.entries(connections.inputs).map(([portName, links]) => [
				portName,
				remapLinks(links),
			]),
		),
		outputs: Object.fromEntries(
			Object.entries(connections.outputs).map(([portName, links]) => [
				portName,
				remapLinks(links),
			]),
		),
	};
};

export const removeNode = (startNodes: NodeMap, nodeId: string): NodeMap => {
	const { [nodeId]: _deletedNode, ...rest } = startNodes;
	const nodes = Object.values(rest).reduce<NodeMap>((obj, node) => {
		obj[node.id] = {
			...node,
			connections: removeConnections(node.connections, nodeId),
		};

		return obj;
	}, {});

	deleteConnectionsByNodeId(nodeId);
	return nodes;
};

const findNodeByType = (
	nodes: NodeMap,
	nodeType: string,
): FlumeNode | undefined =>
	Object.values(nodes).find((node) => node.type === nodeType);

/*
applyDefaultConnections wires demo edges by node type after default nodes exist.
Runs during init so connection endpoints use stable node ids from the start.
*/
export const applyDefaultConnections = (
	nodes: NodeMap,
	defaultConnections: DefaultConnection[],
): NodeMap => {
	let nextNodes = nodes;

	for (const connection of defaultConnections) {
		const outputNode = findNodeByType(nextNodes, connection.output.nodeType);
		const inputNode = findNodeByType(nextNodes, connection.input.nodeType);

		if (!outputNode || !inputNode) {
			continue;
		}

		const existingInputs =
			inputNode.connections?.inputs[connection.input.portName] ?? [];
		const hasValidConnection = existingInputs.some(
			(link) =>
				link.nodeId === outputNode.id &&
				link.portName === connection.output.portName,
		);

		if (hasValidConnection) {
			continue;
		}

		nextNodes = addConnection(
			nextNodes,
			{ nodeId: inputNode.id, portName: connection.input.portName },
			{ nodeId: outputNode.id, portName: connection.output.portName },
		);
	}

	return nextNodes;
};
