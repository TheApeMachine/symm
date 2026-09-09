import type { ObstacleRect } from "#/components/flume/connectionCalculator";
import { connectionId } from "#/components/flume/connectionCalculator";
import type {
	Coordinate,
	FlumeNode,
	NodeMap,
	TransputType,
} from "#/components/flume/types";

export type NodeLayoutEntry = {
	width: number;
	height: number;
};

export type PortLayoutEntry = {
	offsetX: number;
	offsetY: number;
};

export type SpatialIndexSnapshot = {
	nodeLayouts: Map<string, NodeLayoutEntry>;
	portLayouts: Map<string, PortLayoutEntry>;
};

export const createSpatialIndexSnapshot = (): SpatialIndexSnapshot => ({
	nodeLayouts: new Map(),
	portLayouts: new Map(),
});

export const portLayoutKey = (
	nodeId: string,
	portName: string,
	transputType: TransputType,
): string => `${nodeId}|${portName}|${transputType}`;

export const resolveNodePosition = (
	node: FlumeNode,
	positionOverrides?: Record<string, Coordinate>,
): Coordinate => positionOverrides?.[node.id] ?? { x: node.x, y: node.y };

export const obstacleFromNodeLayout = (
	layout: NodeLayoutEntry,
	position: Coordinate,
): ObstacleRect => ({
	left: position.x,
	right: position.x + layout.width,
	top: position.y,
	bottom: position.y + layout.height,
});

export const portCenterFromLayout = (
	position: Coordinate,
	portLayout: PortLayoutEntry,
): Coordinate => ({
	x: position.x + portLayout.offsetX,
	y: position.y + portLayout.offsetY,
});

export const buildObstacleMapFromSpatialIndex = (
	nodes: NodeMap,
	snapshot: SpatialIndexSnapshot,
	positionOverrides?: Record<string, Coordinate>,
): Map<string, ObstacleRect> => {
	const obstacles = new Map<string, ObstacleRect>();

	for (const [nodeId, node] of Object.entries(nodes)) {
		const layout = snapshot.nodeLayouts.get(nodeId);

		if (!layout) {
			continue;
		}

		const position = resolveNodePosition(node, positionOverrides);
		obstacles.set(nodeId, obstacleFromNodeLayout(layout, position));
	}

	return obstacles;
};

export type ResolvedConnection = {
	id: string;
	from: Coordinate;
	to: Coordinate;
	outputNodeId: string;
	inputNodeId: string;
	outputPortName: string;
	inputPortName: string;
};

export const resolveConnectionsFromSpatialIndex = (
	nodes: NodeMap,
	snapshot: SpatialIndexSnapshot,
	positionOverrides?: Record<string, Coordinate>,
): ResolvedConnection[] => {
	const resolved: ResolvedConnection[] = [];

	for (const node of Object.values(nodes)) {
		if (!node.connections?.inputs) {
			continue;
		}

		const inputPosition = resolveNodePosition(node, positionOverrides);

		for (const [inputName, outputs] of Object.entries(
			node.connections.inputs,
		)) {
			const toLayout = snapshot.portLayouts.get(
				portLayoutKey(node.id, inputName, "input"),
			);

			if (!toLayout) {
				continue;
			}

			const to = portCenterFromLayout(inputPosition, toLayout);

			for (const output of outputs) {
				const outputNode = nodes[output.nodeId];

				if (!outputNode) {
					continue;
				}

				const fromLayout = snapshot.portLayouts.get(
					portLayoutKey(output.nodeId, output.portName, "output"),
				);

				if (!fromLayout) {
					continue;
				}

				const outputPosition = resolveNodePosition(
					outputNode,
					positionOverrides,
				);
				const from = portCenterFromLayout(outputPosition, fromLayout);

				resolved.push({
					id: connectionId(output.nodeId, output.portName, node.id, inputName),
					from,
					to,
					outputNodeId: output.nodeId,
					outputPortName: output.portName,
					inputNodeId: node.id,
					inputPortName: inputName,
				});
			}
		}
	}

	return resolved;
};
