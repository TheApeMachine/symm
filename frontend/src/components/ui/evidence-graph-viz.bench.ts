import { bench, describe } from "vitest";
import type { Graph } from "./evidence-graph.types";
import { layoutEvidenceGraph } from "./evidence-graph-viz";

const graph: Graph = {
	symbol: "BTC/USD",
	at: "2026-07-14T12:00:00Z",
	nodes: Array.from({ length: 24 }, (_, index) => ({
		key: `node-${index}`,
		measurement: {
			source: "hawkes",
			metric: "arrival_rate",
			symbol: "BTC/USD",
		},
	})),
	edges: Array.from({ length: 32 }, (_, index) => ({
		from: `node-${index % 24}`,
		to: `node-${(index + 3) % 24}`,
		type: index % 2 === 0 ? "supports" : "contradicts",
		at: "2026-07-14T12:00:00Z",
		observedFrom: "2026-07-14T11:59:00Z",
	})),
};

describe("evidence graph layout", () => {
	bench("layoutEvidenceGraph", () => {
		layoutEvidenceGraph(graph, 960, 640);
	});
});
