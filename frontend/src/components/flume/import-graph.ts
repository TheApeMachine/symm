import {
	fingerprintNodes,
	pipelineGraphCollection,
} from "#/collections/pipeline_graph";
import { hubBaseUrl } from "#/lib/hub";
import { computeNodeRanks } from "./graphLayout";
import { portFamily } from "./port-families";
import type { InputData, NodeMap } from "./types";

export type BackendGraph = {
	id?: string;
	name?: string;
	nodes: Record<
		string,
		{
			id?: string;
			type: string;
			connections?: {
				inputs?: Record<string, Array<{ nodeId: string; portName: string }>>;
				outputs?: Record<string, Array<{ nodeId: string; portName: string }>>;
			};
			inputData?: Record<string, unknown>;
		}
	>;
};

const X_STEP = 360;

/* What a node costs vertically: its header, and a row per port it draws. */
const NODE_HEADER = 96;
const PORT_ROW = 28;
const COLUMN_GAP = 56;

/*
Estimates how tall a node will be drawn.

A node's height is its ports, and a gathering port's numbered slots collapse
into the one port they belong to, so the grid collecting four hundred metrics
is a few rows rather than four hundred. Spacing every node the same distance
apart is what piled the signals on top of each other: they are the tallest
nodes in the graph and were given the same room as a websocket client.
*/
const estimateHeight = (node: NodeMap[string]) => {
	// A sub-graph is closed until someone opens it, and draws no ports while
	// it is: a title and the line saying what it holds.
	if (node.type.startsWith("definition:")) {
		return NODE_HEADER + PORT_ROW;
	}

	const rows = (ports: Record<string, unknown>) =>
		new Set(
			Object.keys(ports).map((port) => portFamily(port) ?? port),
		).size;

	const ports =
		rows(node.connections?.inputs ?? {}) + rows(node.connections?.outputs ?? {});

	return NODE_HEADER + ports * PORT_ROW;
};

/*
Convert a backend declarative JSON graph into a fully-laid-out Flume NodeMap.
Topological ranks place sources on the left and flow rightwards to sinks.
*/
export const convertJSONGraphToFlumeNodes = (
	graph: BackendGraph,
): NodeMap => {
	const flumeNodes: NodeMap = {};

	if (!graph?.nodes || typeof graph.nodes !== "object") {
		return flumeNodes;
	}

	for (const [id, node] of Object.entries(graph.nodes)) {
		flumeNodes[id] = {
			id: node.id || id,
			type: node.type,
			width: 280,
			x: 0,
			y: 0,
			inputData: (node.inputData as InputData) ?? {},
			connections: {
				inputs: node.connections?.inputs ?? {},
				outputs: node.connections?.outputs ?? {},
			},
		};
	}

	const ranks = computeNodeRanks(flumeNodes);
	const maxRank = Math.max(0, ...Array.from(ranks.values()));
	const layers: string[][] = Array.from({ length: maxRank + 1 }, () => []);

	for (const id of Object.keys(flumeNodes)) {
		const r = ranks.get(id) ?? 0;
		layers[r]?.push(id);
	}

	// A rank is packed into a roughly square block rather than one column.
	// Fifteen signal sub-graphs stacked in a line is a ribbon three screens
	// tall; in a block it reads at a glance.
	let column = 0;

	for (const layer of layers) {
		layer.sort((a, b) => a.localeCompare(b));

		const across = Math.ceil(Math.sqrt(layer.length));
		const down = Math.ceil(layer.length / Math.max(1, across));
		const columns: string[][] = Array.from({ length: across }, () => []);

		layer.forEach((nodeId, index) => {
			columns[Math.floor(index / down)].push(nodeId);
		});

		columns.forEach((held, offsetAcross) => {
			const heights = held.map((nodeId) => estimateHeight(flumeNodes[nodeId]));
			const total =
				heights.reduce((sum, height) => sum + height, 0) +
				COLUMN_GAP * Math.max(0, held.length - 1);

			let offsetDown = 120 - total / 2;

			held.forEach((nodeId, index) => {
				flumeNodes[nodeId].x = 100 + (column + offsetAcross) * X_STEP;
				flumeNodes[nodeId].y = offsetDown;
				offsetDown += heights[index] + COLUMN_GAP;
			});
		});

		column += across;
	}

	return flumeNodes;
};

/*
Frames a freshly laid-out graph.

The stage draws its nodes inside a wrapper offset by the negated viewport
translate, whose origin is the centre of the visible area, so the camera sits
over the content when it holds the content's own centre. The viewport is
persisted separately from the nodes, so without this an import lays the graph
out correctly and leaves the camera wherever it was last dragged, which looks
exactly like an empty canvas.
*/
const frameNodes = (nodes: NodeMap) => {
	const placed = Object.values(nodes);

	if (placed.length === 0) {
		return { scale: 1, translate: { x: 0, y: 0 } };
	}

	let left = Number.POSITIVE_INFINITY;
	let right = Number.NEGATIVE_INFINITY;
	let top = Number.POSITIVE_INFINITY;
	let bottom = Number.NEGATIVE_INFINITY;

	for (const node of placed) {
		const width = node.width ?? 0;

		left = Math.min(left, node.x);
		right = Math.max(right, node.x + width);
		top = Math.min(top, node.y);
		bottom = Math.max(bottom, node.y);
	}

	return {
		scale: 1,
		translate: { x: (left + right) / 2, y: (top + bottom) / 2 },
	};
};

/*
Imports a JSON graph into pipelineGraphCollection under targetGraphId.
This updates localStorage synchronously so Flume's usePipelineGraphRow receives it immediately.
*/
export const importJSONGraphToCollection = (
	graph: BackendGraph,
	targetGraphId: string,
	projectId: string | null = null,
	definitionId?: string,
): NodeMap => {
	const nodes = convertJSONGraphToFlumeNodes(graph);
	const source = definitionId
		? {
				definition: definitionId,
				fingerprint: fingerprintNodes(graph.nodes ?? {}),
			}
		: null;

	const existing = pipelineGraphCollection.get(targetGraphId);
	if (existing) {
		pipelineGraphCollection.update(targetGraphId, (draft) => {
			draft.nodes = nodes;
			draft.viewport = frameNodes(nodes);
			draft.source = source;
			draft.updated_at = new Date();
		});
	} else {
		pipelineGraphCollection.insert({
			id: targetGraphId,
			project_id: projectId,
			schema_version: 1,
			nodes,
			comments: {},
			viewport: frameNodes(nodes),
			source,
			updated_at: new Date(),
		});
	}

	return nodes;
};

/*
Reports whether the canvas is showing something other than what the backend
now holds for that definition.

The drawing on the canvas is a working copy. A definition rewired on disk
leaves that copy stale, and nothing about it says so: it keeps drawing the
shape it had when it was first opened.
*/
export const isStaleAgainst = (
	targetGraphId: string,
	definitionId: string,
	graph: BackendGraph,
): boolean => {
	const existing = pipelineGraphCollection.get(targetGraphId);

	if (!existing || Object.keys(existing.nodes ?? {}).length === 0) {
		return true;
	}

	const source = existing.source as
		| { definition: string; fingerprint: string }
		| null
		| undefined;

	if (!source || source.definition !== definitionId) {
		return true;
	}

	return source.fingerprint !== fingerprintNodes(graph.nodes ?? {});
};

/*
Fetches a definition by ID from the backend and imports it into pipelineGraphCollection.
*/
export const fetchAndImportDefinition = async (
	definitionId: string,
	targetGraphId: string,
	projectId: string | null = null,
): Promise<NodeMap> => {
	const sanitized = definitionId.replace(/:/g, "_");
	const response = await fetch(`${hubBaseUrl()}/workbench/signals/${sanitized}`);

	if (!response.ok) {
		throw new Error(
			`Failed to fetch signal definition ${definitionId}: ${response.statusText}`,
		);
	}

	const graph: BackendGraph = await response.json();
	return importJSONGraphToCollection(
		graph,
		targetGraphId,
		projectId,
		definitionId,
	);
};

/*
Brings the canvas up to date with the definition it claims to be showing,
and reports whether it had to.

An import is what the canvas does when it has nothing, but a drawing already
on it can be just as wrong: the definition behind it may have been rewired
since. Fetching first and comparing means an unchanged definition leaves the
drawing, and the work on it, exactly as it was.
*/
export const reconcileDefinition = async (
	definitionId: string,
	targetGraphId: string,
	projectId: string | null = null,
): Promise<boolean> => {
	const sanitized = definitionId.replace(/:/g, "_");
	const response = await fetch(`${hubBaseUrl()}/workbench/signals/${sanitized}`);

	if (!response.ok) {
		throw new Error(
			`Failed to fetch signal definition ${definitionId}: ${response.statusText}`,
		);
	}

	const graph: BackendGraph = await response.json();

	if (!isStaleAgainst(targetGraphId, definitionId, graph)) {
		return false;
	}

	importJSONGraphToCollection(graph, targetGraphId, projectId, definitionId);
	return true;
};
