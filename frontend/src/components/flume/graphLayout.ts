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
