import { Graph as RenderGraph } from "#/components/graph/core/graph";

export type MarketGraphNode = {
	id: string;
	symbol?: string;
	source?: string;
	kind?: string;
	value?: number;
	confidence?: number;
	at?: string;
	metadata?: Record<string, unknown>;
};

export type MarketGraphEdge = {
	from: string;
	to: string;
	relation?: string;
	weight?: number;
	confidence?: number;
	at?: string;
	reason?: string;
};

export type MarketGraphFrame = {
	at?: string;
	nodes?: Record<string, MarketGraphNode>;
	edges?: MarketGraphEdge[];
	adjacency?: Record<string, string[]>;
};

const listeners = new Set<() => void>();
let retainedFrame: MarketGraphFrame | null = null;
let retainedGraph: RenderGraph | null = null;
let displayedStructureKey = "";
let pendingStructureKey = "";

const isRecord = (value: unknown): value is Record<string, unknown> =>
	value !== null && typeof value === "object";

const isMarketGraphFrame = (value: unknown): value is MarketGraphFrame => {
	if (!isRecord(value)) {
		return false;
	}

	return isRecord(value.nodes) && Array.isArray(value.edges);
};

const graphFrames = (value: unknown): MarketGraphFrame[] => {
	if (Array.isArray(value)) {
		return value.filter(isMarketGraphFrame);
	}

	if (isMarketGraphFrame(value)) {
		return [value];
	}

	if (!isRecord(value)) {
		return [];
	}

	return Object.values(value).filter(isMarketGraphFrame);
};

export const adaptGraph = (frame: MarketGraphFrame): RenderGraph => {
	const graph = new RenderGraph({
		epoch: "at",
		epochFormat: "YYYY-MM-DDTHH:mm:ssZ",
		source: "from",
		target: "to",
	});

	for (const node of Object.values(frame.nodes ?? {})) {
		if (typeof node.id !== "string" || node.id === "") {
			continue;
		}

		graph.addNode(node.id, {
			...node.metadata,
			at: node.at,
			confidence: node.confidence,
			kind: node.kind,
			source: node.source,
			symbol: node.symbol,
			value: node.value,
		});
	}

	for (const edge of frame.edges ?? []) {
		if (typeof edge.from !== "string" || typeof edge.to !== "string") {
			continue;
		}

		graph.addEdge(edge.from, edge.to, {
			...edge,
			from: edge.from,
			to: edge.to,
		});
	}

	return graph;
};

export const graphStructureKey = (frame: MarketGraphFrame): string => {
	const nodeIds = Object.values(frame.nodes ?? {})
		.map((node) => node.id)
		.filter((id) => typeof id === "string" && id !== "")
		.sort()
		.join(",");
	const edgeKeys = (frame.edges ?? [])
		.map((edge) => `${edge.from}->${edge.to}:${edge.relation ?? ""}`)
		.sort()
		.join(",");

	return `${nodeIds}|${edgeKeys}`;
};

export type GraphFramePlan = "initialize" | "refresh" | "stage";

export const graphFramePlan = (
	displayedKey: string,
	frame: MarketGraphFrame,
): GraphFramePlan => {
	if (displayedKey === "") {
		return "initialize";
	}

	return displayedKey === graphStructureKey(frame) ? "refresh" : "stage";
};

const notify = (): void => {
	for (const listener of listeners) {
		listener();
	}
};

const install = (frame: MarketGraphFrame): void => {
	displayedStructureKey = graphStructureKey(frame);
	pendingStructureKey = "";
	retainedGraph = adaptGraph(frame);
};

export const readGraphSurface = () => ({
	frame: retainedFrame,
	graph: retainedGraph,
	topologyPending: pendingStructureKey !== "",
});

export const subscribeGraphSurface = (listener: () => void): (() => void) => {
	listeners.add(listener);

	return () => listeners.delete(listener);
};

export const applyPendingGraphSurface = (): void => {
	if (retainedFrame === null || pendingStructureKey === "") {
		return;
	}

	install(retainedFrame);
	notify();
};

/*
paintGraphSurface refreshes inspection data on every frame but replaces the GPU
graph only on initial load. Structural changes wait for explicit user sync.
*/
export const paintGraphSurface = (value: unknown): void => {
	const frame = graphFrames(value).at(-1) ?? null;

	if (frame === null) {
		return;
	}

	retainedFrame = frame;
	const plan = graphFramePlan(displayedStructureKey, frame);

	if (plan === "initialize") {
		install(frame);
	}

	pendingStructureKey = plan === "stage" ? graphStructureKey(frame) : "";
	notify();
};
