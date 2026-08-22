import { describe, expect, it } from "vitest";
import type { DiagnosticsFrame, QueueSnapshot } from "#/collections/types";
import {
	buildDiagnosticsGraph,
	routeIntersectsPlacement,
} from "./diagnostics-graph";

const FRAME: DiagnosticsFrame = {
	at_ns: 10_000_000_000,
	started_ns: 1_000_000_000,
	stages: [],
	errors: [],
	pass: { state: "idle" },
	queues: [
		{
			name: "ingress.tickers",
			kind: "ingress",
			writers: ["crypto"],
			readers: ["correlation", "cvd", "leadlag"],
			depth: 20,
			high_water: 30,
		},
		{
			name: "measurements",
			kind: "rail",
			writers: ["correlation", "cvd", "leadlag"],
			readers: ["category", "manifold", "graph"],
			depth: 200,
			high_water: 250,
		},
		{
			name: "derived.category",
			kind: "derived",
			writers: ["category"],
			readers: ["graph", "cognition"],
			depth: 4,
			high_water: 8,
		},
		{
			name: "derived.graph",
			kind: "derived",
			writers: ["graph"],
			readers: ["planner"],
			depth: 1,
			high_water: 2,
		},
	],
	hops: [
		{
			from: "graph",
			to: "planner",
			count: 2,
			total_ns: 8_000_000,
			last_ns: 5_000_000,
			max_ns: 5_000_000,
		},
	],
};

const queue = (
	name: string,
	writers: string[],
	readers: string[],
): QueueSnapshot => ({
	name,
	kind: "rail",
	writers,
	readers,
	depth: 12_345,
	high_water: 20_000,
});

const FULL_FRAME: DiagnosticsFrame = {
	...FRAME,
	queues: [
		queue(
			"ingress.tickers",
			["crypto"],
			["correlation", "cvd", "leadlag", "liquidity", "pumpdump", "sentiment"],
		),
		queue(
			"ingress.trades",
			["crypto"],
			["cvd", "depthflow", "exhaustion", "hawkes", "pumpdump", "toxicity"],
		),
		queue("ingress.level3", ["crypto"], ["toxicity", "pumpdump"]),
		queue(
			"measurements",
			[
				"correlation",
				"cvd",
				"depthflow",
				"exhaustion",
				"hawkes",
				"leadlag",
				"liquidity",
				"pumpdump",
				"sentiment",
				"toxicity",
			],
			["category", "manifold", "graph"],
		),
		queue("derived.category", ["category"], ["graph", "cognition"]),
		queue("derived.causal", ["causal"], ["graph", "causal"]),
		queue("derived.cognition", ["cognition"], ["graph"]),
		queue("derived.graph", ["graph"], ["planner", "graph"]),
		queue("derived.resonance", ["resonance"], ["causal", "graph"]),
		queue("decisions", ["planner"], ["audit"]),
		queue("positions", ["desk"], ["audit"]),
		queue(
			"ui.dashboard",
			[
				"category",
				"manifold",
				"causal",
				"cognition",
				"graph",
				"resonance",
				"planner",
				"desk",
			],
			["hub"],
		),
		queue(
			"ui.manifold",
			["manifold", "resonance", "diagnostics"],
			["webrtc-hub"],
		),
		queue("desk.ticker", ["websocket-api"], ["desk"]),
		queue("desk.executions", ["websocket-api"], ["desk"]),
	],
	hops: [
		{
			from: "crypto",
			to: "measurements",
			count: 10,
			total_ns: 1_000,
			last_ns: 100,
		},
		{
			from: "measurements",
			to: "category",
			count: 10,
			total_ns: 2_000,
			last_ns: 200,
		},
		{
			from: "category",
			to: "causal",
			count: 10,
			total_ns: 3_000,
			last_ns: 300,
		},
		{ from: "causal", to: "graph", count: 10, total_ns: 4_000, last_ns: 400 },
		{ from: "graph", to: "planner", count: 10, total_ns: 5_000, last_ns: 500 },
		{ from: "planner", to: "mcts", count: 10, total_ns: 6_000, last_ns: 600 },
		{
			from: "mcts",
			to: "allocation",
			count: 10,
			total_ns: 7_000,
			last_ns: 700,
		},
		{
			from: "allocation",
			to: "desk",
			count: 10,
			total_ns: 8_000,
			last_ns: 800,
		},
	],
};

describe("buildDiagnosticsGraph", () => {
	it("routes every edge around every unrelated node clearance box", () => {
		const graph = buildDiagnosticsGraph(FULL_FRAME);

		for (const edge of graph.edges) {
			for (const placement of graph.placements.values()) {
				if (placement.id === edge.from || placement.id === edge.to) {
					continue;
				}

				expect(
					routeIntersectsPlacement(edge.points, placement),
					`${edge.id} crossed ${placement.id}`,
				).toBe(false);
			}
		}
	});

	it("distributes contacts evenly across a shared node side", () => {
		const graph = buildDiagnosticsGraph(FRAME);
		const contacts = graph.edges
			.filter((edge) => edge.kind === "write" && edge.to === "measurements")
			.map((edge) => edge.points.at(-1)?.x ?? 0)
			.sort((first, second) => first - second);

		expect(contacts).toHaveLength(3);
		expect(new Set(contacts).size).toBe(3);
		expect(contacts[1] - contacts[0]).toBeCloseTo(contacts[2] - contacts[1], 8);
	});

	it("carries the observed average handoff latency onto its edge", () => {
		const firstGraph = buildDiagnosticsGraph(FRAME);
		const firstHop = firstGraph.edges.find(
			(edge) =>
				edge.kind === "hop" && edge.from === "graph" && edge.to === "planner",
		);
		const nextGraph = buildDiagnosticsGraph({
			...FRAME,
			hops: FRAME.hops?.map((hop) => ({ ...hop, total_ns: 12_000_000 })),
		});
		const nextHop = nextGraph.edges.find(
			(edge) =>
				edge.kind === "hop" && edge.from === "graph" && edge.to === "planner",
		);

		expect(firstHop?.latencyNs).toBe(4_000_000);
		expect(nextHop?.latencyNs).toBe(6_000_000);
		expect(nextHop?.points).toBe(firstHop?.points);
		expect(nextHop?.labelPoint.x).toBeTypeOf("number");
		expect(nextHop?.labelPoint.y).toBeTypeOf("number");
	});

	it("generates input and output connection ports for every edge", () => {
		const graph = buildDiagnosticsGraph(FULL_FRAME);

		expect(graph.ports.length).toBe(graph.edges.length * 2);

		for (const edge of graph.edges) {
			const outPort = graph.ports.find(
				(port) => port.edgeId === edge.id && port.kind === "out",
			);
			const inPort = graph.ports.find(
				(port) => port.edgeId === edge.id && port.kind === "in",
			);

			expect(outPort).toBeDefined();
			expect(inPort).toBeDefined();
			expect(outPort?.point).toEqual(edge.points[0]);
			expect(inPort?.point).toEqual(edge.points[edge.points.length - 1]);
		}
	});

	it("assigns distinct non-overlapping horizontal corridor lanes for parallel fan-out edges", () => {
		const graph = buildDiagnosticsGraph(FULL_FRAME);
		const categoryEdge = graph.edges.find(
			(edge) => edge.from === "measurements" && edge.to === "category",
		);
		const manifoldEdge = graph.edges.find(
			(edge) => edge.from === "measurements" && edge.to === "manifold",
		);

		expect(categoryEdge).toBeDefined();
		expect(manifoldEdge).toBeDefined();

		const categoryTrunkY = categoryEdge?.points[2]?.y;
		const manifoldTrunkY = manifoldEdge?.points[2]?.y;

		expect(categoryTrunkY).toBeDefined();
		expect(manifoldTrunkY).toBeDefined();
		expect(categoryTrunkY).not.toEqual(manifoldTrunkY);
	});
});


