import { createStore } from "@tanstack/store";
import { Graph as RenderGraph } from "#/components/graph/core/graph";

export type MarketGraphNode = {
	id: string;
	symbol?: string;
	source?: string;
	kind?: string;
	value?: number;
	strength?: number;
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

export type GraphSurfaceState = {
	frame: MarketGraphFrame | null;
	graph: RenderGraph | null;
	displayedStructureKey: string;
	pendingStructureKey: string;
};

const graphSurfaceStore = createStore<GraphSurfaceState>({
	frame: null,
	graph: null,
	displayedStructureKey: "",
	pendingStructureKey: "",
});

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

	const connected = new Set<string>();

	for (const edge of frame.edges ?? []) {
		if (
			typeof edge.from !== "string" ||
			typeof edge.to !== "string" ||
			edge.from === "" ||
			edge.to === ""
		) {
			continue;
		}

		connected.add(edge.from);
		connected.add(edge.to);
	}

	for (const node of Object.values(frame.nodes ?? {})) {
		if (typeof node.id !== "string" || !connected.has(node.id)) {
			continue;
		}

		graph.addNode(node.id, {
			...node.metadata,
			at: node.at,
			confidence: node.confidence,
			kind: node.kind,
			source: node.source,
			strength: node.strength,
			symbol: node.symbol,
			value: node.value,
		});
	}

	for (const edge of frame.edges ?? []) {
		if (!connected.has(edge.from) || !connected.has(edge.to)) {
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

export const renderedNodeIds = (frame: MarketGraphFrame): Set<string> => {
	return new Set(Object.keys(adaptGraph(frame).nodes));
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

const install = (frame: MarketGraphFrame): void => {
	graphSurfaceStore.setState((state) => ({
		...state,
		displayedStructureKey: graphStructureKey(frame),
		pendingStructureKey: "",
		graph: adaptGraph(frame),
	}));
};

export const readGraphSurface = () => {
	const state = graphSurfaceStore.state;

	return {
		frame: state.frame,
		graph: state.graph,
		topologyPending: state.pendingStructureKey !== "",
	};
};

export const subscribeGraphSurface = (listener: () => void): (() => void) => {
	const subscription = graphSurfaceStore.subscribe(listener);

	return () => subscription.unsubscribe();
};

export const applyPendingGraphSurface = (): void => {
	const state = graphSurfaceStore.state;

	if (state.frame === null || state.pendingStructureKey === "") {
		return;
	}

	install(state.frame);
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

	graphSurfaceStore.setState((state) => {
		const structureKey = graphStructureKey(frame);
		const plan = graphFramePlan(state.displayedStructureKey, frame);

		if (plan === "initialize") {
			return {
				frame,
				graph: adaptGraph(frame),
				displayedStructureKey: structureKey,
				pendingStructureKey: "",
			};
		}

		return {
			...state,
			frame,
			pendingStructureKey: plan === "stage" ? structureKey : "",
		};
	});
};
