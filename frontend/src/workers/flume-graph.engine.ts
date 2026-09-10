import {
	calculateEdgePath,
	type EdgeRoutingMode,
	type ObstacleRect,
} from "#/components/flume/connectionCalculator";
import {
	buildObstacleMapFromSpatialIndex,
	type NodeLayoutEntry,
	type PortLayoutEntry,
	portLayoutKey,
	resolveConnectionsFromSpatialIndex,
	type SpatialIndexSnapshot,
} from "#/components/flume/spatial-index";
import type {
	Coordinate,
	NodeMap,
	TransputType,
} from "#/components/flume/types";
import type {
	ConnectionDescriptor,
	ConnectionPathResult,
	DragUpdateResult,
	GraphSnapshot,
	RecalculateResult,
} from "#/workers/flume-graph.types";

const snapshotFromEngine = (
	nodeLayouts: Map<string, NodeLayoutEntry>,
	portLayouts: Map<string, PortLayoutEntry>,
): SpatialIndexSnapshot => ({
	nodeLayouts,
	portLayouts,
});

const computeOrthogonalPath = (
	from: Coordinate,
	to: Coordinate,
	allObstaclesById: Map<string, ObstacleRect>,
	outputNodeId: string,
	inputNodeId: string,
	allObstacles: ObstacleRect[],
): string => {
	const obstaclesHorizontal = Array.from(allObstaclesById.entries())
		.filter(([nodeId]) => nodeId !== outputNodeId && nodeId !== inputNodeId)
		.map(([, rect]) => rect);

	return calculateEdgePath(
		"orthogonal",
		from,
		to,
		allObstacles,
		obstaclesHorizontal,
	);
};

/*
FlumeGraphEngine owns the serializable graph snapshot and recomputes edge
paths from node positions plus registered port offsets — no DOM reads.
*/
export class FlumeGraphEngine {
	private nodes: NodeMap = {};
	private routingMode: EdgeRoutingMode = "smooth";
	private nodeLayouts = new Map<string, NodeLayoutEntry>();
	private portLayouts = new Map<string, PortLayoutEntry>();
	private dragNodeId: string | null = null;
	private dragPosition: Coordinate | null = null;

	/*
	setGraph replaces only the node topology, preserving any port and
	node layouts that have already been measured by the main thread.
	Use this for incremental topology updates.

	The main thread already plain-JSON-clones nodes before postMessage to
	escape the collection's reactive proxies, so we treat the incoming
	value as our own owned data — no second clone.
	*/
	setGraph(nodes: NodeMap): void {
		this.nodes = nodes;

		// Drop layouts for nodes that no longer exist; keep the rest.
		const liveIds = new Set(Object.keys(this.nodes));

		for (const nodeId of Array.from(this.nodeLayouts.keys())) {
			if (!liveIds.has(nodeId)) {
				this.nodeLayouts.delete(nodeId);
			}
		}

		for (const key of Array.from(this.portLayouts.keys())) {
			const nodeId = key.split("|")[0];

			if (!liveIds.has(nodeId)) {
				this.portLayouts.delete(key);
			}
		}
	}

	setRoutingMode(routingMode: EdgeRoutingMode): void {
		this.routingMode = routingMode;
	}

	setNodeLayout(nodeId: string, width: number, height: number): void {
		if (width <= 0 || height <= 0) {
			return;
		}

		this.nodeLayouts.set(nodeId, { width, height });
	}

	setPortLayout(
		nodeId: string,
		portName: string,
		transputType: TransputType,
		offsetX: number,
		offsetY: number,
	): void {
		this.portLayouts.set(portLayoutKey(nodeId, portName, transputType), {
			offsetX,
			offsetY,
		});
	}

	beginDrag(nodeId: string): void {
		const node = this.nodes[nodeId];

		if (!node) {
			return;
		}

		this.dragNodeId = nodeId;
		this.dragPosition = { x: node.x, y: node.y };
	}

	updateDrag(nodeId: string, x: number, y: number): DragUpdateResult {
		this.dragNodeId = nodeId;
		this.dragPosition = { x, y };

		return {
			paths: this.computePaths(),
			dragPosition: { x, y },
		};
	}

	endDrag(nodeId: string, x: number, y: number): ConnectionPathResult[] {
		const node = this.nodes[nodeId];

		if (node) {
			node.x = x;
			node.y = y;
		}

		this.dragNodeId = null;
		this.dragPosition = null;

		return this.computePaths();
	}

	computePaths(): ConnectionPathResult[] {
		return this.recalculate().paths;
	}

	recalculate(): RecalculateResult {
		const positionOverrides = this.buildPositionOverrides();
		const snapshot = snapshotFromEngine(this.nodeLayouts, this.portLayouts);
		const allObstaclesById = buildObstacleMapFromSpatialIndex(
			this.nodes,
			snapshot,
			positionOverrides,
		);
		const allObstacles = Array.from(allObstaclesById.values());
		const resolved = resolveConnectionsFromSpatialIndex(
			this.nodes,
			snapshot,
			positionOverrides,
		);

		const paths: ConnectionPathResult[] = [];
		const roster: ConnectionDescriptor[] = [];

		for (const connection of resolved) {
			roster.push({
				id: connection.id,
				outputNodeId: connection.outputNodeId,
				outputPortName: connection.outputPortName,
				inputNodeId: connection.inputNodeId,
				inputPortName: connection.inputPortName,
			});

			if (this.routingMode === "orthogonal") {
				paths.push({
					id: connection.id,
					d: computeOrthogonalPath(
						connection.from,
						connection.to,
						allObstaclesById,
						connection.outputNodeId,
						connection.inputNodeId,
						allObstacles,
					),
				});
				continue;
			}

			paths.push({
				id: connection.id,
				d: calculateEdgePath(
					this.routingMode,
					connection.from,
					connection.to,
					allObstacles,
					allObstacles,
				),
			});
		}

		return { paths, roster };
	}

	getSnapshot(): GraphSnapshot {
		return {
			nodes: structuredClone(this.nodes),
			routingMode: this.routingMode,
			nodeLayouts: Array.from(this.nodeLayouts.entries()).map(
				([nodeId, layout]) => ({
					nodeId,
					width: layout.width,
					height: layout.height,
				}),
			),
			portLayouts: Array.from(this.portLayouts.entries()).map(
				([key, layout]) => {
					const [nodeId, portName, transputType] = key.split("|");

					return {
						nodeId,
						portName,
						transputType: transputType as TransputType,
						offsetX: layout.offsetX,
						offsetY: layout.offsetY,
					};
				},
			),
		};
	}

	private buildPositionOverrides(): Record<string, Coordinate> | undefined {
		if (!this.dragNodeId || !this.dragPosition) {
			return undefined;
		}

		return { [this.dragNodeId]: this.dragPosition };
	}
}
