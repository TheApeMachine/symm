import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import { hubBaseUrl } from "#/lib/hub";
import { computeNodeRanks } from "./graphLayout";
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
const Y_STEP = 240;

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

	layers.forEach((layer, rank) => {
		layer.sort((a, b) => a.localeCompare(b));
		const count = layer.length;
		layer.forEach((nodeId, idx) => {
			flumeNodes[nodeId].x = 100 + rank * X_STEP;
			flumeNodes[nodeId].y = 120 + (idx - (count - 1) / 2) * Y_STEP;
		});
	});

	return flumeNodes;
};

/*
Imports a JSON graph into pipelineGraphCollection under targetGraphId.
This updates localStorage synchronously so Flume's usePipelineGraphRow receives it immediately.
*/
export const importJSONGraphToCollection = (
	graph: BackendGraph,
	targetGraphId: string,
	projectId: string | null = null,
): NodeMap => {
	const nodes = convertJSONGraphToFlumeNodes(graph);

	const existing = pipelineGraphCollection.get(targetGraphId);
	if (existing) {
		pipelineGraphCollection.update(targetGraphId, (draft) => {
			draft.nodes = nodes;
			draft.updated_at = new Date();
		});
	} else {
		pipelineGraphCollection.insert({
			id: targetGraphId,
			project_id: projectId,
			schema_version: 1,
			nodes,
			comments: {},
			viewport: { scale: 1, translate: { x: 0, y: 0 } },
			updated_at: new Date(),
		});
	}

	return nodes;
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
	return importJSONGraphToCollection(graph, targetGraphId, projectId);
};
