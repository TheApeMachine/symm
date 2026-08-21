import { bench, describe } from "vitest";
import type { DiagnosticsFrame, QueueSnapshot } from "#/collections/types";
import { buildDiagnosticsGraph } from "./diagnostics-graph";

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

const FRAME: DiagnosticsFrame = {
	at_ns: 10_000_000_000,
	started_ns: 1_000_000_000,
	stages: [],
	errors: [],
	pass: { state: "running" },
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
	bench("refreshes the complete live topology with stable routing", () => {
		buildDiagnosticsGraph(FRAME);
	});
});
