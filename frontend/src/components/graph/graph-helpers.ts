import type { Edge, Graph, GraphData, Node } from "./core/graph";
import { Graph as GraphClass } from "./core/graph";

/*
isDescendant returns true if candidate sits at or below root in the
dotted-name hierarchy. The root "" (empty string) matches everything.
"__model__" is treated as the universal root.
*/
export const isDescendant = (candidate: string, root: string): boolean => {
	if (root === "" || root === "__model__") return true;
	if (candidate === root) return true;
	return candidate.startsWith(`${root}.`);
};

/*
childrenOf returns the direct children of root: nodes whose name is
exactly one segment deeper. Used by the inspector to render the
drill-down list.
*/
export const childrenOf = (graph: Graph, root: string): string[] => {
	const prefix = root === "" || root === "__model__" ? "" : `${root}.`;
	const out: string[] = [];

	for (const name of Object.keys(graph.nodes)) {
		if (name === root) continue;
		if (!name.startsWith(prefix)) continue;
		const tail = prefix === "" ? name : name.slice(prefix.length);
		if (tail.length === 0) continue;
		if (tail.includes(".")) continue;
		out.push(name);
	}

	return out.sort();
};

/*
buildSubgraph returns a new Graph containing only nodes whose name sits
at or below root, with their edges restricted to that subset. Node and
edge ids are re-numbered densely so the renderer's textures stay
compact.
*/
export const buildSubgraph = (source: Graph, root: string): Graph => {
	if (root === "" || root === "__model__") {
		const copy = new GraphClass();
		copy.loadFromData(source.toJSON());
		return copy;
	}

	const includedNames = new Set<string>();

	for (const name of Object.keys(source.nodes)) {
		if (isDescendant(name, root)) {
			includedNames.add(name);
		}
	}

	const nameToNewId = new Map<string, number>();
	let nextNodeId = 0;
	const newNodes: Record<string, Node> = {};

	for (const name of includedNames) {
		const original = source.nodes[name];
		if (!original) continue;
		const id = nextNodeId++;
		nameToNewId.set(name, id);
		newNodes[name] = { id, edges: [], data: original.data };
	}

	const newEdges: Record<string, Edge> = {};
	let nextEdgeId = 0;

	for (const [key, edge] of Object.entries(source.edges)) {
		if (!includedNames.has(edge.source) || !includedNames.has(edge.target)) {
			continue;
		}

		const sourceId = nameToNewId.get(edge.source);
		const targetId = nameToNewId.get(edge.target);
		if (sourceId === undefined || targetId === undefined) continue;

		const newEdge: Edge = {
			source: edge.source,
			target: edge.target,
			id: nextEdgeId++,
			data: edge.data,
		};

		newEdges[key] = newEdge;
		newNodes[edge.source].edges.push(targetId);
		newNodes[edge.target].edges.push(sourceId);
	}

	const data: GraphData = {
		nodes: newNodes,
		edges: newEdges,
		settings: source.settings,
	};

	const subgraph = new GraphClass();
	subgraph.loadFromData(data);
	return subgraph;
};

/*
searchNodes returns names ranked by simple substring + path-depth
heuristic so leaves with the most unique tail win over noisy parents.
*/
export const searchNodes = (
	graph: Graph,
	query: string,
	limit = 40,
): string[] => {
	const trimmed = query.trim().toLowerCase();
	if (trimmed.length === 0) return [];

	const matches: Array<{ name: string; score: number }> = [];

	for (const name of Object.keys(graph.nodes)) {
		const lower = name.toLowerCase();
		const tail = (name.split(".").at(-1) ?? "").toLowerCase();

		let score = 0;
		if (tail === trimmed) score += 100;
		else if (tail.startsWith(trimmed)) score += 50;
		else if (tail.includes(trimmed)) score += 20;
		else if (lower.includes(trimmed)) score += 5;

		if (score > 0) {
			matches.push({ name, score });
		}
	}

	matches.sort((a, b) => b.score - a.score || a.name.localeCompare(b.name));
	return matches.slice(0, limit).map((entry) => entry.name);
};
