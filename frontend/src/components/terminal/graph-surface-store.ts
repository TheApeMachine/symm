import { createStore } from "@tanstack/store";
import { Graph as RenderGraph } from "#/components/graph/core/graph";
import { GraphEdge } from "#/providers/telemetry/telemetry/graph-edge";
import { GraphFrame } from "#/providers/telemetry/telemetry/graph-frame";
import { GraphMetadata } from "#/providers/telemetry/telemetry/graph-metadata";
import { GraphNode } from "#/providers/telemetry/telemetry/graph-node";
import { NamedNumber } from "#/providers/telemetry/telemetry/named-number";
import { NamedString } from "#/providers/telemetry/telemetry/named-string";

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

const nodeObj = new GraphNode();
const edgeObj = new GraphEdge();
const metadataObj = new GraphMetadata();
const namedNumberObj = new NamedNumber();
const namedStringObj = new NamedString();

/*
graphMetadataFromFlatBuffer decodes the backend's named string/number metadata
table (timescale, epoch) into a plain record for the inspection panel.
*/
const graphMetadataFromFlatBuffer = (
	meta: GraphMetadata | null,
): Record<string, unknown> | undefined => {
	if (!meta) {
		return undefined;
	}

	const entries: Record<string, unknown> = {};

	for (let i = 0; i < meta.stringsLength(); i++) {
		const entry = meta.strings(i, namedStringObj);
		const name = entry?.name();

		if (name !== null && name !== undefined) {
			entries[name] = entry?.value() ?? "";
		}
	}

	for (let i = 0; i < meta.numbersLength(); i++) {
		const entry = meta.numbers(i, namedNumberObj);
		const name = entry?.name();

		if (name !== null && name !== undefined) {
			entries[name] = entry?.value();
		}
	}

	return entries;
};

export const graphFrameFromFlatBuffer = (fb: GraphFrame | null): MarketGraphFrame | null => {
	if (!fb) return null;

	const nodes: Record<string, MarketGraphNode> = {};
	for (let i = 0; i < fb.nodesLength(); i++) {
		const n = fb.nodes(i, nodeObj);
		if (n && n.id()) {
			const id = n.id()!;
			nodes[id] = {
				id,
				symbol: n.symbol() ?? undefined,
				source: n.source() ?? undefined,
				kind: n.kind() ?? undefined,
				value: n.value(),
				strength: n.strength(),
				confidence: n.confidence(),
				at: String(n.at()),
				metadata: graphMetadataFromFlatBuffer(n.metadata(metadataObj)),
			};
		}
	}

	const edges: MarketGraphEdge[] = [];
	for (let i = 0; i < fb.edgesLength(); i++) {
		const e = fb.edges(i, edgeObj);
		if (e && e.from() && e.to()) {
			edges.push({
				from: e.from()!,
				to: e.to()!,
				relation: e.relation() ?? undefined,
				weight: e.weight(),
				confidence: e.confidence(),
				at: String(e.at()),
				reason: e.reason() ?? undefined,
			});
		}
	}

	return {
		at: String(fb.at()),
		nodes,
		edges,
	};
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
	const connected = new Set<string>();
	const edgeKeys: string[] = [];

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
		edgeKeys.push(`${edge.from}->${edge.to}:${edge.relation ?? ""}`);
	}

	const nodeIds = Object.values(frame.nodes ?? {})
		.map((node) => node.id)
		.filter((id) => typeof id === "string" && id !== "" && connected.has(id))
		.sort()
		.join(",");

	edgeKeys.sort();

	return `${nodeIds}|${edgeKeys.join(",")}`;
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

	if (state.frame === null) {
		return;
	}

	install(state.frame);
};

/*
paintGraphSurface refreshes inspection data on every frame but replaces the GPU
graph only on initial load. Structural changes wait for explicit user sync.
*/
export const paintGraphSurface = (value: unknown): void => {
	let frame: MarketGraphFrame | null = null;
	if (value instanceof GraphFrame) {
		frame = graphFrameFromFlatBuffer(value);
	} else if (value && typeof value === "object" && "getLast" in value && typeof (value as any).getLast === "function") {
		frame = graphFrameFromFlatBuffer((value as any).getLast());
	} else if (value && typeof value === "object" && "nodes" in value && "edges" in value) {
		frame = value as MarketGraphFrame;
	}

	if (frame === null) {
		return;
	}

	graphSurfaceStore.setState((state) => {
		const structureKey = graphStructureKey(frame!);
		const plan = graphFramePlan(state.displayedStructureKey, frame!);

		if (plan === "initialize") {
			return {
				frame,
				graph: adaptGraph(frame!),
				displayedStructureKey: structureKey,
				pendingStructureKey: "",
			};
		}

		const isDiverged = structureKey !== state.displayedStructureKey;
		const nextPending = isDiverged ? structureKey : state.pendingStructureKey;

		return {
			...state,
			frame,
			pendingStructureKey: nextPending,
		};
	});
};

