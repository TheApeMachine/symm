/*
Reorders and repositions nodes for simple pipeline previews. Consumers call
{@link dispatchGraphLayout} after the user selects a non-freeform preset; topological
ordering follows edges producer → consumer.
*/
import type { NodeActions } from "#/components/flume/nodes-actions";
import type { NodeMap } from "#/components/flume/types";

/** How automatic arrangement places nodes relative to dependency order. */
export type GraphLayoutMode =
	| "freeform"
	| "horizontalPipeline"
	| "verticalPipeline";

const HORIZ_STEP = 300;
/** Vertical gap between dependency ranks (nodes are tall — match ~card + chart header). */
const VERT_RANK_GAP = 360;
/** Horizontal spacing between nodes that share the same rank (parallel branches). */
const VERT_PARALLEL_GAP = 320;

/*
Computes dependency depth (longest path from any source). Nodes with no incoming graph
edges are rank 0; each hop along an edge increases rank by one.
*/
export function computeNodeRanks(nodes: NodeMap): Map<string, number> {
	const ranks = new Map<string, number>();
	for (const id of Object.keys(nodes)) {
		ranks.set(id, 0);
	}

	let changed = true;
	let iterations = 0;
	const limit = Math.max(Object.keys(nodes).length, 1) + 8;
	while (changed && iterations < limit) {
		iterations++;
		changed = false;
		for (const id of Object.keys(nodes)) {
			const inputs = nodes[id]?.connections?.inputs;
			if (!inputs) continue;
			for (const outs of Object.values(inputs)) {
				for (const link of outs) {
					const pred = link.nodeId;
					if (!ranks.has(pred)) continue;
					const next = (ranks.get(pred) ?? 0) + 1;
					if (next > (ranks.get(id) ?? 0)) {
						ranks.set(id, next);
						changed = true;
					}
				}
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

/** Repositions nodes for pipeline layouts (no-op for {@link GraphLayoutMode.freeform}). */
export function dispatchGraphLayout(
	mode: GraphLayoutMode,
	nodeMap: NodeMap,
	actions: NodeActions,
) {
	if (mode === "freeform") return;

	if (mode === "verticalPipeline") {
		const ranks = computeNodeRanks(nodeMap);
		const maxRank = Math.max(0, ...ranks.values());
		const layers: string[][] = Array.from({ length: maxRank + 1 }, () => []);

		for (const id of Object.keys(nodeMap)) {
			const r = ranks.get(id) ?? 0;
			layers[r]?.push(id);
		}
		for (const layer of layers) {
			layer.sort((a, b) => a.localeCompare(b));
		}

		const updates: Array<{ nodeId: string; x: number; y: number }> = [];
		layers.forEach((layer, rank) => {
			const n = layer.length;
			layer.forEach((nodeId, i) => {
				const x = n <= 1 ? 0 : (i - (n - 1) / 2) * VERT_PARALLEL_GAP;
				const y = rank * VERT_RANK_GAP;
				updates.push({ nodeId, x, y });
			});
		});
		actions.applyNodeCoordinates(updates);
		return;
	}

	const order = topologicalSortNodeIds(nodeMap);
	actions.applyNodeCoordinates(
		order.map((nodeId, i) => ({ nodeId, x: i * HORIZ_STEP, y: 0 })),
	);
}
