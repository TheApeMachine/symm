import { describe, expect, it } from "vitest";
import {
	graphTopologyKey,
	layoutEvidenceGraph,
} from "#/components/terminal/evidence-graph-viz";
import type { GraphFrame } from "#/types/thesis";

const sampleGraph = (): GraphFrame => ({
	symbol: "BTC/USD",
	at: "2026-07-14T12:00:00Z",
	nodes: [
		{
			key: "node-a",
			measurement: {
				source: "hawkes",
				metric: "arrival_rate",
				symbol: "BTC/USD",
			},
		},
		{
			key: "node-b",
			measurement: { source: "fluid", metric: "strength", symbol: "BTC/USD" },
		},
	],
	edges: [
		{
			from: "node-a",
			to: "node-b",
			type: "supports",
			at: "2026-07-14T12:00:00Z",
			observedFrom: "2026-07-14T11:59:00Z",
		},
	],
});

describe("layoutEvidenceGraph", () => {
	it("assigns positions for every graph node inside the canvas bounds", () => {
		const positions = layoutEvidenceGraph(sampleGraph(), 640, 420);

		expect(positions.size).toBe(2);
		expect(positions.get("node-a")?.x).toBeGreaterThan(0);
		expect(positions.get("node-b")?.y).toBeGreaterThan(0);
	});

	it("keeps node positions stable when only the frame timestamp changes", () => {
		const graph = sampleGraph();
		const reordered: GraphFrame = {
			...graph,
			at: "2026-07-14T12:01:00Z",
			nodes: [...graph.nodes].reverse(),
		};
		const first = layoutEvidenceGraph(graph, 640, 420);
		const second = layoutEvidenceGraph(reordered, 640, 420);

		expect(second.get("node-a")).toEqual(first.get("node-a"));
		expect(second.get("node-b")).toEqual(first.get("node-b"));
	});
});

describe("graphTopologyKey", () => {
	it("ignores frame timestamps while node and edge identity stay fixed", () => {
		const graph = sampleGraph();
		const updated: GraphFrame = {
			...graph,
			at: "2026-07-14T12:05:00Z",
			nodes: [...graph.nodes].reverse(),
		};

		expect(graphTopologyKey(updated)).toBe(graphTopologyKey(graph));
	});
});
