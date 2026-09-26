/*
Reorders and repositions nodes for simple pipeline previews. Consumers call
{@link dispatchGraphLayout} after the user selects a non-freeform preset; topological
ordering follows edges producer → consumer.
*/

import type { NodeActions } from "#/components/flume/nodes-actions";
import { portFamily } from "#/components/flume/port-families";
import {
	portLayoutKey,
	type SpatialIndexSnapshot,
} from "#/components/flume/spatial-index";
import type { NodeMap } from "#/components/flume/types";

/** How automatic arrangement places nodes relative to dependency order. */
export type GraphLayoutMode =
	| "freeform"
	| "horizontalPipeline"
	| "verticalPipeline"
	| "orthogonal";

export const NODE_HEADER = 96;
export const PORT_ROW = 28;
export const GRID_CELL_SIZE = 32;
export const NODE_WIDTH = 320;
export const HORIZONTAL_CORRIDOR = 192;
export const X_STEP = NODE_WIDTH + HORIZONTAL_CORRIDOR;
export const MIN_VERTICAL_GAP = 64;
export const START_X = 96;
export const START_Y = 128;

const HORIZ_STEP = 300;
/** Horizontal spacing between nodes that share the same rank (parallel branches). */
const VERT_PARALLEL_GAP = 320;

/** Estimates drawn height of a node based on its spatial layout, DOM height, or structure. */
export const estimateNodeHeight = (
	node: NodeMap[string],
	spatialIndex?: SpatialIndexSnapshot,
): number => {
	if (!node) {
		return NODE_HEADER;
	}

	const indexed = node.id ? spatialIndex?.nodeLayouts.get(node.id) : undefined;
	if (indexed && indexed.height > 0) {
		return indexed.height;
	}

	if (typeof node.height === "number" && node.height > 0) {
		return node.height;
	}

	if (node.id && typeof document !== "undefined") {
		const element = document.querySelector(
			`[data-flume-component="node"][data-node-id="${node.id}"], [data-node-id="${node.id}"]`,
		);
		if (element) {
			const clientHeight = (element as HTMLElement).clientHeight;
			const rectHeight = Math.round(element.getBoundingClientRect().height);
			const measuredHeight = rectHeight > 0 ? rectHeight : clientHeight;
			if (measuredHeight > 0) {
				return measuredHeight;
			}
		}
	}

	if (node.type?.startsWith("definition:")) {
		return 128;
	}

	let totalHeight = NODE_HEADER;
	const inputData = node.inputData ?? {};
	const connectedInputs = new Set(Object.keys(node.connections?.inputs ?? {}));

	for (const [key, controlVal] of Object.entries(inputData)) {
		if (connectedInputs.has(key)) {
			totalHeight += PORT_ROW;
			continue;
		}

		const val = (controlVal as { value?: unknown })?.value ?? controlVal;
		if (typeof val === "boolean") {
			totalHeight += 32;
			continue;
		}

		if (
			Array.isArray(val) ||
			(typeof val === "string" && (val.length > 40 || val.includes("\n")))
		) {
			totalHeight += 96;
			continue;
		}

		totalHeight += 48;
	}

	for (const inputKey of connectedInputs) {
		if (!(inputKey in inputData)) {
			totalHeight += PORT_ROW;
		}
	}

	const outputs = Object.keys(node.connections?.outputs ?? {});
	const outputCount = outputs.length;
	totalHeight += outputCount * PORT_ROW;

	return Math.max(totalHeight, NODE_HEADER);
};

/** Calculates vertical offset of a port relative to the top of the node card. */
export const estimatePortOffsetY = (
	node: NodeMap[string],
	portName: string,
	isOutput: boolean,
	spatialIndex?: SpatialIndexSnapshot,
): number => {
	if (!node) {
		return NODE_HEADER / 2;
	}

	const transputType = isOutput ? "output" : "input";

	if (node.id && spatialIndex) {
		const key = portLayoutKey(node.id, portName, transputType);
		const layout = spatialIndex.portLayouts.get(key);
		if (layout && layout.offsetY > 0) {
			return layout.offsetY;
		}

		const family = portFamily(portName);
		if (family) {
			const familyLayout = spatialIndex.portLayouts.get(
				portLayoutKey(node.id, family, transputType),
			);
			if (familyLayout && familyLayout.offsetY > 0) {
				return familyLayout.offsetY;
			}
		}
	}

	if (node.id && typeof document !== "undefined") {
		const nodeElement = document.querySelector(
			`[data-flume-component="node"][data-node-id="${node.id}"], [data-node-id="${node.id}"]`,
		);
		if (nodeElement) {
			const family = portFamily(portName);
			const portElement =
				nodeElement.querySelector(
					`[data-port-name="${portName}"][data-port-transput-type="${transputType}"]`,
				) ??
				nodeElement.querySelector(`[data-port-name="${portName}"]`) ??
				(family
					? nodeElement.querySelector(`[data-port-name="${family}"]`)
					: null);

			if (portElement) {
				const nodeRect = nodeElement.getBoundingClientRect();
				const portRect = portElement.getBoundingClientRect();
				const scale = nodeRect.width / (node.width || NODE_WIDTH) || 1;
				const offsetY =
					(portRect.top + portRect.height / 2 - nodeRect.top) / scale;
				if (offsetY > 0) {
					return Math.round(offsetY);
				}
			}
		}
	}

	const inputs = Object.keys(node.connections?.inputs ?? {});
	const outputs = Object.keys(node.connections?.outputs ?? {});
	const inputData = node.inputData ?? {};
	const targetFamily = portFamily(portName) ?? portName;

	let inputSectionHeight = 0;
	const inputFamilies = Array.from(
		new Set(inputs.map((name) => portFamily(name) ?? name)),
	);

	for (const [key, controlVal] of Object.entries(inputData)) {
		if (inputs.includes(key)) {
			inputSectionHeight += PORT_ROW;
			continue;
		}

		const val = (controlVal as { value?: unknown })?.value ?? controlVal;
		if (typeof val === "boolean") {
			inputSectionHeight += 32;
			continue;
		}

		if (
			Array.isArray(val) ||
			(typeof val === "string" && (val.length > 40 || val.includes("\n")))
		) {
			inputSectionHeight += 96;
			continue;
		}

		inputSectionHeight += 48;
	}

	if (isOutput) {
		const outputFamilies = Array.from(
			new Set(outputs.map((name) => portFamily(name) ?? name)),
		);
		const outIndex = Math.max(0, outputFamilies.indexOf(targetFamily));
		return NODE_HEADER + inputSectionHeight + (outIndex + 0.5) * PORT_ROW;
	}

	const inIndex = Math.max(0, inputFamilies.indexOf(targetFamily));
	return NODE_HEADER + (inIndex + 0.5) * PORT_ROW;
};

/*
Collects the producer -> consumer edges, dropping the ones that close a cycle.
Feedback is real in this system: the grid hands the signals the values it was
holding when the evaluation began, and the signals hand their results back. An
edge that reaches a node already open on the walk is that feedback, and it
constrains no position, so the walk records it and moves on.
*/
function forwardEdges(nodes: NodeMap): Map<string, Set<string>> {
	const successors = new Map<string, Set<string>>();
	for (const id of Object.keys(nodes)) {
		successors.set(id, new Set());
	}

	for (const id of Object.keys(nodes)) {
		const inputs = nodes[id]?.connections?.inputs;

		if (!inputs) continue;

		for (const incoming of Object.values(inputs)) {
			for (const link of incoming) {
				if (!successors.has(link.nodeId)) continue;

				successors.get(link.nodeId)?.add(id);
			}
		}
	}

	const open = new Set<string>();
	const settled = new Set<string>();

	const walk = (root: string) => {
		const stack: Array<{ id: string; next: string[]; at: number }> = [
			{ id: root, next: [...(successors.get(root) ?? [])], at: 0 },
		];
		open.add(root);

		while (stack.length > 0) {
			const frame = stack[stack.length - 1];

			if (frame.at >= frame.next.length) {
				open.delete(frame.id);
				settled.add(frame.id);
				stack.pop();
				continue;
			}

			const succ = frame.next[frame.at];
			frame.at++;

			if (open.has(succ)) {
				successors.get(frame.id)?.delete(succ);
				continue;
			}

			if (settled.has(succ)) continue;

			open.add(succ);
			stack.push({ id: succ, next: [...(successors.get(succ) ?? [])], at: 0 });
		}
	};

	// Walking out of the true sources first breaks each loop at the edge that
	// actually reaches backwards. Starting anywhere else breaks it at whichever
	// edge the walk happened to enter on, which strands that node at rank 0.
	const incoming = new Set<string>();
	for (const outs of successors.values()) {
		for (const succ of outs) {
			incoming.add(succ);
		}
	}

	const roots = Object.keys(nodes).filter((id) => !incoming.has(id));

	for (const id of [...roots, ...Object.keys(nodes)]) {
		if (settled.has(id)) continue;

		walk(id);
	}

	return successors;
}

/*
Computes dependency depth (longest path from any source) over the acyclic
edges. Nodes with no incoming edge are rank 0; each hop along an edge increases
rank by one.
*/
export function computeNodeRanks(nodes: NodeMap): Map<string, number> {
	const ranks = new Map<string, number>();
	for (const id of Object.keys(nodes)) {
		ranks.set(id, 0);
	}

	const successors = forwardEdges(nodes);
	const remaining = new Map<string, number>();
	for (const id of Object.keys(nodes)) {
		remaining.set(id, 0);
	}
	for (const outs of successors.values()) {
		for (const succ of outs) {
			remaining.set(succ, (remaining.get(succ) ?? 0) + 1);
		}
	}

	const queue = Object.keys(nodes).filter((id) => remaining.get(id) === 0);

	while (queue.length > 0) {
		const id = queue.shift() as string;

		for (const succ of successors.get(id) ?? []) {
			const next = (ranks.get(id) ?? 0) + 1;

			if (next > (ranks.get(succ) ?? 0)) {
				ranks.set(succ, next);
			}

			const left = (remaining.get(succ) ?? 0) - 1;
			remaining.set(succ, left);

			if (left === 0) {
				queue.push(succ);
			}
		}
	}

	return ranks;
}

/** Returns node ids ordered producer → consumer (Kahn topological sort); leftovers append if cyclic. */
export function topologicalSortNodeIds(nodes: NodeMap): string[] {
	const ids = Object.keys(nodes);
	if (ids.length === 0) return [];

	const inDegree = new Map<string, number>();
	const successors = new Map<string, Set<string>>();
	for (const id of ids) {
		inDegree.set(id, 0);
		successors.set(id, new Set());
	}

	for (const id of ids) {
		const inputs = nodes[id]?.connections?.inputs;
		if (!inputs) continue;
		for (const outgoing of Object.values(inputs)) {
			for (const link of outgoing) {
				const pred = link.nodeId;
				if (!successors.has(pred) || !inDegree.has(id)) continue;
				const set = successors.get(pred);
				if (!set?.has(id)) {
					set?.add(id);
					inDegree.set(id, (inDegree.get(id) ?? 0) + 1);
				}
			}
		}
	}

	const queue = ids
		.filter((id) => (inDegree.get(id) ?? 0) === 0)
		.sort((a, b) => a.localeCompare(b));
	const sorted: string[] = [];

	while (queue.length > 0) {
		const n = queue.shift() as string;
		sorted.push(n);
		const outs = successors.get(n);
		if (!outs) continue;
		for (const succ of [...outs].sort((a, b) => a.localeCompare(b))) {
			const nextDeg = (inDegree.get(succ) ?? 0) - 1;
			inDegree.set(succ, nextDeg);
			if (nextDeg === 0) {
				queue.push(succ);
				queue.sort((a, b) => a.localeCompare(b));
			}
		}
	}

	if (sorted.length < ids.length) {
		const rest = ids
			.filter((id) => !sorted.includes(id))
			.sort((a, b) => a.localeCompare(b));
		sorted.push(...rest);
	}
	return sorted;
}

/**
 * Optimizes node positioning for orthogonal edge routing:
 * 1. Partitions nodes into topological ranks (left to right).
 * 2. Minimizes edge crossings via multi-pass barycenter sweeps.
 * 3. Horizontally aligns input and output ports (for 0-bend and 2-bend orthogonal routing).
 * 4. Snaps all coordinates to the 32px occupancy grid with non-overlapping corridors.
 */
export function optimizeOrthogonalLayout(
	nodes: NodeMap,
	spatialIndex?: SpatialIndexSnapshot,
): Array<{ nodeId: string; x: number; y: number }> {
	const nodeIds = Object.keys(nodes);
	if (nodeIds.length === 0) {
		return [];
	}

	const ranks = computeNodeRanks(nodes);
	const maxRank = Math.max(0, ...Array.from(ranks.values()));
	const layers: string[][] = Array.from({ length: maxRank + 1 }, () => []);

	for (const nodeId of nodeIds) {
		const nodeRank = ranks.get(nodeId) ?? 0;
		layers[nodeRank]?.push(nodeId);
	}

	// Crossing reduction: multi-pass barycenter sweeps
	const sweepRounds = 2;
	for (let round = 0; round < sweepRounds; round++) {
		// Forward sweep: order each layer by average position of predecessors
		for (let rankIndex = 1; rankIndex <= maxRank; rankIndex++) {
			const prevLayer = layers[rankIndex - 1];
			const currentLayer = layers[rankIndex];

			const barycenters = new Map<string, number>();
			for (const nodeId of currentLayer) {
				const inputs = nodes[nodeId]?.connections?.inputs ?? {};
				let predecessorSum = 0;
				let predecessorCount = 0;

				for (const links of Object.values(inputs)) {
					for (const link of links) {
						const predecessorIndex = prevLayer.indexOf(link.nodeId);
						if (predecessorIndex >= 0) {
							predecessorSum += predecessorIndex;
							predecessorCount++;
						}
					}
				}

				const barycenterValue =
					predecessorCount > 0
						? predecessorSum / predecessorCount
						: currentLayer.indexOf(nodeId);
				barycenters.set(nodeId, barycenterValue);
			}

			currentLayer.sort((nodeA, nodeB) => {
				const scoreA = barycenters.get(nodeA) ?? 0;
				const scoreB = barycenters.get(nodeB) ?? 0;
				if (scoreA !== scoreB) {
					return scoreA - scoreB;
				}

				return nodeA.localeCompare(nodeB);
			});
		}

		// Backward sweep: order each layer by average position of successors
		for (let rankIndex = maxRank - 1; rankIndex >= 0; rankIndex--) {
			const nextLayer = layers[rankIndex + 1];
			const currentLayer = layers[rankIndex];

			const barycenters = new Map<string, number>();
			for (const nodeId of currentLayer) {
				const outputs = nodes[nodeId]?.connections?.outputs ?? {};
				let successorSum = 0;
				let successorCount = 0;

				for (const links of Object.values(outputs)) {
					for (const link of links) {
						const successorIndex = nextLayer.indexOf(link.nodeId);
						if (successorIndex >= 0) {
							successorSum += successorIndex;
							successorCount++;
						}
					}
				}

				const barycenterValue =
					successorCount > 0
						? successorSum / successorCount
						: currentLayer.indexOf(nodeId);
				barycenters.set(nodeId, barycenterValue);
			}

			currentLayer.sort((nodeA, nodeB) => {
				const scoreA = barycenters.get(nodeA) ?? 0;
				const scoreB = barycenters.get(nodeB) ?? 0;
				if (scoreA !== scoreB) {
					return scoreA - scoreB;
				}

				return nodeA.localeCompare(nodeB);
			});
		}
	}

	// Position assignments
	const positions = new Map<string, { coordX: number; coordY: number }>();

	for (let rankIndex = 0; rankIndex <= maxRank; rankIndex++) {
		const currentLayer = layers[rankIndex];
		const layerCoordX = START_X + rankIndex * X_STEP;

		if (rankIndex === 0) {
			let currentCoordY = START_Y;
			for (const nodeId of currentLayer) {
				const snappedCoordY =
					Math.ceil(currentCoordY / GRID_CELL_SIZE) * GRID_CELL_SIZE;
				positions.set(nodeId, { coordX: layerCoordX, coordY: snappedCoordY });

				const nodeHeight = estimateNodeHeight(nodes[nodeId], spatialIndex);
				currentCoordY = snappedCoordY + nodeHeight + MIN_VERTICAL_GAP;
			}
			continue;
		}

		// Calculate target vertical positions to align input ports with predecessor output ports
		const targetPositions: Array<{ nodeId: string; targetY: number }> = [];

		for (const nodeId of currentLayer) {
			const inputs = nodes[nodeId]?.connections?.inputs ?? {};
			let targetSum = 0;
			let targetCount = 0;

			for (const [inputPortName, links] of Object.entries(inputs)) {
				const inPortOffsetY = estimatePortOffsetY(
					nodes[nodeId],
					inputPortName,
					false,
					spatialIndex,
				);

				for (const link of links) {
					const predecessorPos = positions.get(link.nodeId);
					if (!predecessorPos) {
						continue;
					}

					const outPortOffsetY = estimatePortOffsetY(
						nodes[link.nodeId],
						link.portName,
						true,
						spatialIndex,
					);
					const idealNodeY =
						predecessorPos.coordY + outPortOffsetY - inPortOffsetY;
					targetSum += idealNodeY;
					targetCount++;
				}
			}

			let computedTargetY = START_Y;
			if (targetCount > 0) {
				computedTargetY = targetSum / targetCount;
			}

			targetPositions.push({ nodeId, targetY: computedTargetY });
		}

		// Sort targetPositions by targetY ascending to preserve natural top-to-bottom flow
		targetPositions.sort((itemA, itemB) => {
			if (itemA.targetY !== itemB.targetY) {
				return itemA.targetY - itemB.targetY;
			}
			return (
				currentLayer.indexOf(itemA.nodeId) - currentLayer.indexOf(itemB.nodeId)
			);
		});

		// Enforce non-overlapping vertical placement with minimum gaps
		let runningCoordY = START_Y;
		for (let itemIndex = 0; itemIndex < targetPositions.length; itemIndex++) {
			const item = targetPositions[itemIndex];
			const candidateY = Math.max(item.targetY, runningCoordY);
			const snappedCoordY =
				Math.ceil(candidateY / GRID_CELL_SIZE) * GRID_CELL_SIZE;

			positions.set(item.nodeId, {
				coordX: layerCoordX,
				coordY: snappedCoordY,
			});

			const nodeHeight = estimateNodeHeight(nodes[item.nodeId], spatialIndex);
			runningCoordY = snappedCoordY + nodeHeight + MIN_VERTICAL_GAP;
		}
	}

	const updates: Array<{ nodeId: string; x: number; y: number }> = [];
	for (const [nodeId, pos] of positions.entries()) {
		updates.push({ nodeId, x: pos.coordX, y: pos.coordY });
	}

	return updates;
}

/** Repositions nodes for pipeline layouts (no-op for {@link GraphLayoutMode.freeform}). */
export function dispatchGraphLayout(
	mode: GraphLayoutMode,
	nodeMap: NodeMap,
	actions: NodeActions,
	spatialIndex?: SpatialIndexSnapshot,
) {
	if (mode === "freeform") return;

	if (mode === "orthogonal") {
		const updates = optimizeOrthogonalLayout(nodeMap, spatialIndex);
		actions.applyNodeCoordinates(updates);
		return;
	}

	if (mode === "verticalPipeline") {
		const ranks = computeNodeRanks(nodeMap);
		const maxRank = Math.max(0, ...ranks.values());
		const layers: string[][] = Array.from({ length: maxRank + 1 }, () => []);

		for (const id of Object.keys(nodeMap)) {
			const rankValue = ranks.get(id) ?? 0;
			layers[rankValue]?.push(id);
		}
		for (const layer of layers) {
			layer.sort((nodeA, nodeB) => nodeA.localeCompare(nodeB));
		}

		const updates: Array<{ nodeId: string; x: number; y: number }> = [];
		let currentCoordY = 0;
		for (let rankIndex = 0; rankIndex <= maxRank; rankIndex++) {
			const layer = layers[rankIndex];
			const layerCount = layer.length;
			let maxLayerHeight = 0;
			for (let nodeIndex = 0; nodeIndex < layerCount; nodeIndex++) {
				const nodeId = layer[nodeIndex];
				const coordX =
					layerCount <= 1
						? 0
						: (nodeIndex - (layerCount - 1) / 2) * VERT_PARALLEL_GAP;
				updates.push({ nodeId, x: coordX, y: currentCoordY });
				const nodeHeight = estimateNodeHeight(nodeMap[nodeId], spatialIndex);
				if (nodeHeight > maxLayerHeight) {
					maxLayerHeight = nodeHeight;
				}
			}
			currentCoordY += maxLayerHeight + MIN_VERTICAL_GAP;
		}
		actions.applyNodeCoordinates(updates);
		return;
	}

	const order = topologicalSortNodeIds(nodeMap);
	actions.applyNodeCoordinates(
		order.map((nodeId, nodeIndex) => ({
			nodeId,
			x: nodeIndex * HORIZ_STEP,
			y: 0,
		})),
	);
}
