import { describe, expect, it } from "vitest";
import {
	adaptGraph,
	graphFramePlan,
	graphStructureKey,
	type MarketGraphEdge,
	type MarketGraphFrame,
	type MarketGraphNode,
} from "./graph-surface-store";

const frame = (): MarketGraphFrame & {
	nodes: Record<string, MarketGraphNode>;
	edges: MarketGraphEdge[];
} => ({
	at: "2026-08-04T10:00:00Z",
	nodes: {
		alpha: { id: "alpha", value: 0.2, confidence: 0.8 },
		beta: { id: "beta", value: -0.1, confidence: 0.6 },
	},
	edges: [{ from: "alpha", to: "beta", relation: "supports", weight: 0.4 }],
});

describe("adaptGraph", () => {
	it("renders the relational graph without isolated conclusions", () => {
		const current = frame();
		current.nodes.isolated = { id: "isolated", value: 1 };

		const graph = adaptGraph(current);

		expect(graph.getNodeCount()).toBe(2);
		expect(graph.getEdgeCount()).toBe(1);
		expect(graph.nodes.isolated).toBeUndefined();
		expect(graph.nodes.alpha?.data[0]?.value).toBe(0.2);
	});
});

describe("graphFramePlan", () => {
	it("refreshes live values without replacing an unchanged topology", () => {
		const initial = frame();
		const updated = frame();
		updated.at = "2026-08-04T10:00:01Z";
		updated.nodes.alpha.value = 0.9;

		if (updated.edges[0]) {
			updated.edges[0].weight = 0.7;
		}

		const displayedKey = graphStructureKey(initial);

		expect(graphFramePlan(displayedKey, updated)).toBe("refresh");
	});

	it("stages node and relation changes for an explicit topology sync", () => {
		const initial = frame();
		const nodeAdded = frame();
		nodeAdded.nodes.gamma = { id: "gamma" };
		const relationChanged = frame();

		if (relationChanged.edges[0]) {
			relationChanged.edges[0].relation = "contradicts";
		}

		const displayedKey = graphStructureKey(initial);

		expect(graphFramePlan(displayedKey, nodeAdded)).toBe("stage");
		expect(graphFramePlan(displayedKey, relationChanged)).toBe("stage");
	});

	it("initializes the first usable graph frame", () => {
		expect(graphFramePlan("", frame())).toBe("initialize");
	});
});

describe("graphStructureKey", () => {
	it("is independent of map, edge, and metadata ordering", () => {
		const left = frame();
		const right: MarketGraphFrame = {
			nodes: {
				beta: { id: "beta", metadata: { latest: true } },
				alpha: { id: "alpha", value: 99 },
			},
			edges: [
				{ from: "alpha", to: "beta", relation: "supports", confidence: 0.1 },
			],
		};

		expect(graphStructureKey(right)).toBe(graphStructureKey(left));
	});
});
