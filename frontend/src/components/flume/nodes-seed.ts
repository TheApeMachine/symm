import type { NodeActionsEnv } from "#/components/flume/nodes-actions";
import {
	getDefaultData,
	reconcileNodes,
} from "#/components/flume/nodes-helpers";
import { addConnection as addConnectionPure } from "#/components/flume/nodes-mutations";
import type { FlumeNode, NodeMap } from "#/components/flume/types";
import { createFlumeId } from "#/components/flume/utilities";

/*
nodes-seed builds the initial NodeMap for a fresh graph row. It runs
once at insert time — useNodesState.seed() calls into here when no
collection row exists for the given graphId. After that, all topology
changes go through nodes-actions.

This is separate from nodes-actions because the seed path doesn't go
through the collection write path; it produces a final NodeMap that
becomes the row's initial nodes field.
*/

const synthesizeNode = (
	params: { x: number; y: number; nodeType: string; id?: string },
	env: NodeActionsEnv,
): FlumeNode | undefined => {
	const { x, y, nodeType, id } = params;
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

	if (nodeTypeDef.root) {
		newNode.root = true;
	}

	if (nodeTypeDef.defaultSubGraph) {
		newNode.subGraph = nodeTypeDef.defaultSubGraph;
	}

	return newNode;
};

export const buildInitialNodes = ({
	initialNodes = {},
	defaultNodes = [],
	defaultConnections = [],
	env,
}: {
	initialNodes?: NodeMap;
	defaultNodes?: ReadonlyArray<{ type: string; x?: number; y?: number }>;
	defaultConnections?: ReadonlyArray<{
		output: { nodeType: string; portName: string };
		input: { nodeType: string; portName: string };
	}>;
	env: NodeActionsEnv;
}): NodeMap => {
	const reconciled = reconcileNodes(
		initialNodes,
		env.nodeTypes,
		env.portTypes,
		env.context,
	);

	let withDefaults: NodeMap = { ...reconciled };

	for (const dNode of defaultNodes) {
		const alreadyHas = Object.values(initialNodes).some(
			(node) => node.type === dNode.type,
		);

		if (alreadyHas || !env.nodeTypes[dNode.type]) {
			continue;
		}

		const newNode = synthesizeNode(
			{ x: dNode.x ?? 0, y: dNode.y ?? 0, nodeType: dNode.type },
			env,
		);

		if (!newNode) {
			continue;
		}

		withDefaults[newNode.id] = newNode;
	}

	const findByType = (nodeType: string): FlumeNode | undefined =>
		Object.values(withDefaults).find((node) => node.type === nodeType);

	for (const connection of defaultConnections) {
		const outputNode = findByType(connection.output.nodeType);
		const inputNode = findByType(connection.input.nodeType);

		if (!outputNode || !inputNode) {
			continue;
		}

		const existing =
			inputNode.connections?.inputs[connection.input.portName] ?? [];
		const alreadyConnected = existing.some(
			(link) =>
				link.nodeId === outputNode.id &&
				link.portName === connection.output.portName,
		);

		if (alreadyConnected) {
			continue;
		}

		withDefaults = addConnectionPure(
			withDefaults,
			{ nodeId: inputNode.id, portName: connection.input.portName },
			{ nodeId: outputNode.id, portName: connection.output.portName },
		);
	}

	return withDefaults;
};
