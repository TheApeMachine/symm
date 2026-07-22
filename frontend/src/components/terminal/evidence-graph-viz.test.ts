import { describe, expect, it } from "vitest";
import {
	buildScene,
	edgeControlPoint,
	type GraphScene,
	graphVisualKey,
	hitTest,
	layoutEvidenceGraph,
	nodeKind,
	nodeLabel,
	pairKey,
	reciprocalPairs,
} from "#/components/terminal/evidence-graph-viz";
import type {
	GraphEdge,
	Graph,
	GraphNode,
} from "#/types/thesis";

/*
categoryGraph models the real category-centered shape: two measurements draw
Supports/Contradicts edges to a category hypothesis hub, and a directed causal
Conditions edge links two concept nodes.
*/
const categoryGraph = (): Graph => ({
	symbol: "BTC/USD",
	at: "2026-07-14T12:00:00Z",
	nodes: [
		{
			key: "depthflow/depth_flow/loaded_score/book_imbalance//BTC/USD/1",
			kind: "measurement",
			measurement: {
				source: "depthflow",
				metric: "loaded_score",
				subject: "book_imbalance",
				symbol: "BTC/USD",
				normalized: 0.8,
				validity: { state: "valid" },
				at: "2026-07-14T12:00:00Z",
			},
		},
		{
			key: "depthflow/depth_flow/neutral_score/book_imbalance//BTC/USD/1",
			kind: "measurement",
			measurement: {
				source: "depthflow",
				metric: "neutral_score",
				subject: "book_imbalance",
				symbol: "BTC/USD",
				normalized: 0.5,
				validity: { state: "valid" },
				at: "2026-07-14T12:00:00Z",
			},
		},
		{
			key: "category/loaded_imbalance",
			kind: "category",
			category: "loaded_imbalance",
			measurement: {
				source: "category",
				metric: "loaded_imbalance",
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
			},
		},
		{
			key: "causal/buy_sell_arrival_intensity_imbalance",
			kind: "concept",
			measurement: {
				source: "causal",
				metric: "buy_sell_arrival_intensity_imbalance",
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
			},
		},
		{
			key: "causal/next_l3_epoch_mid_log_return",
			kind: "concept",
			measurement: {
				source: "causal",
				metric: "next_l3_epoch_mid_log_return",
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
			},
		},
	],
	edges: [
		{
			from: "depthflow/depth_flow/loaded_score/book_imbalance//BTC/USD/1",
			to: "category/loaded_imbalance",
			type: "supports",
			at: "2026-07-14T12:00:00Z",
			observedFrom: "2026-07-14T11:59:00Z",
		},
		{
			from: "depthflow/depth_flow/neutral_score/book_imbalance//BTC/USD/1",
			to: "category/loaded_imbalance",
			type: "contradicts",
			at: "2026-07-14T12:00:00Z",
			observedFrom: "2026-07-14T11:59:00Z",
		},
		{
			from: "causal/buy_sell_arrival_intensity_imbalance",
			to: "causal/next_l3_epoch_mid_log_return",
			type: "conditions",
			at: "2026-07-14T12:00:00Z",
			observedFrom: "2026-07-14T12:00:00Z",
		},
	],
});

describe("nodeKind", () => {
	it("classifies by explicit wire kind", () => {
		const graph = categoryGraph();

		expect(nodeKind(graph.nodes[0] as GraphNode)).toBe("measurement");
		expect(nodeKind(graph.nodes[2] as GraphNode)).toBe("category");
		expect(nodeKind(graph.nodes[3] as GraphNode)).toBe("concept");
	});

	it("falls back to descriptive source when kind is absent", () => {
		const legacyCategory: GraphNode = {
			key: "category/turbulent",
			measurement: { source: "category", metric: "turbulent" },
		};
		const legacyMeasurement: GraphNode = {
			key: "hawkes/x",
			measurement: { source: "hawkes", metric: "arrival_rate" },
		};

		expect(nodeKind(legacyCategory)).toBe("category");
		expect(nodeKind(legacyMeasurement)).toBe("measurement");
	});
});

describe("nodeLabel", () => {
	it("labels categories by subject and measurements by source/metric", () => {
		const graph = categoryGraph();

		expect(nodeLabel(graph.nodes[2] as GraphNode)).toBe("loaded_imbalance");
		expect(nodeLabel(graph.nodes[0] as GraphNode)).toBe(
			"depthflow/loaded_score",
		);
	});
});

describe("layoutEvidenceGraph", () => {
	it("assigns an in-bounds position to every node", () => {
		const graph = categoryGraph();
		const positions = layoutEvidenceGraph(graph, 640, 420);

		expect(positions.size).toBe(graph.nodes.length);

		for (const node of graph.nodes) {
			const position = positions.get(node.key);

			expect(position).toBeDefined();
			expect(position?.x).toBeGreaterThanOrEqual(0);
			expect(position?.x).toBeLessThanOrEqual(640);
			expect(position?.y).toBeGreaterThanOrEqual(0);
			expect(position?.y).toBeLessThanOrEqual(420);
		}
	});

	it("places hypothesis hubs nearer the center than unattached measurements", () => {
		const graph = categoryGraph();
		const positions = layoutEvidenceGraph(graph, 640, 420);
		const center = { x: 320, y: 210 };
		const distance = (key: string) => {
			const position = positions.get(key);

			return position === undefined
				? Number.POSITIVE_INFINITY
				: Math.hypot(position.x - center.x, position.y - center.y);
		};

		// The category hub should sit closer to center than the loaded_score
		// measurement clustered around it.
		expect(distance("category/loaded_imbalance")).toBeLessThan(
			distance("depthflow/depth_flow/loaded_score/book_imbalance//BTC/USD/1"),
		);
	});

	it("is deterministic across identical frames", () => {
		const first = layoutEvidenceGraph(categoryGraph(), 640, 420);
		const second = layoutEvidenceGraph(categoryGraph(), 640, 420);

		for (const [key, position] of first) {
			expect(second.get(key)).toEqual(position);
		}
	});

	it("keeps a node's slot when only its per-tick MeasurementKey changes", () => {
		const before = categoryGraph();
		// Simulate the next tick: same nodes/edges by identity, but the
		// measurement keys carry a new timestamp suffix (as MeasurementKey does).
		const rekey = (key: string) =>
			key.startsWith("depthflow/") ? key.replace(/\/1$/, "/2") : key;
		const after: Graph = {
			...before,
			at: "2026-07-14T12:00:01Z",
			nodes: before.nodes.map((node) => ({ ...node, key: rekey(node.key) })),
			edges: before.edges.map((edge) => ({
				...edge,
				from: rekey(edge.from),
				to: rekey(edge.to),
			})),
		};

		const first = layoutEvidenceGraph(before, 640, 420);
		const second = layoutEvidenceGraph(after, 640, 420);
		const loadedBefore = first.get(
			"depthflow/depth_flow/loaded_score/book_imbalance//BTC/USD/1",
		);
		const loadedAfter = second.get(
			"depthflow/depth_flow/loaded_score/book_imbalance//BTC/USD/2",
		);

		expect(loadedAfter).toEqual(loadedBefore);
	});

	it("keeps hub slots when an unrelated hub appears next tick", () => {
		const before = categoryGraph();
		const withExtraHub: Graph = {
			...before,
			nodes: [
				...before.nodes,
				{
					key: "category/turbulent",
					kind: "category",
					category: "turbulent",
					measurement: { source: "category", metric: "turbulent" },
				},
			],
		};

		const first = layoutEvidenceGraph(before, 640, 420);
		const second = layoutEvidenceGraph(withExtraHub, 640, 420);

		// The pre-existing category hub must not swing when a new hub joins.
		expect(second.get("category/loaded_imbalance")).toEqual(
			first.get("category/loaded_imbalance"),
		);
	});
});

describe("graphVisualKey", () => {
	it("changes when an edge type changes", () => {
		const supports = categoryGraph();
		const contradicts: Graph = {
			...supports,
			edges: supports.edges.map((edge, index) =>
				index === 0 ? { ...edge, type: "leads" } : edge,
			),
		};

		expect(graphVisualKey(contradicts)).not.toBe(graphVisualKey(supports));
	});

	it("ignores frame timestamp churn", () => {
		const before = categoryGraph();
		const after: Graph = { ...before, at: "2026-07-14T12:05:00Z" };

		expect(graphVisualKey(after)).toBe(graphVisualKey(before));
	});

	it("ignores MeasurementKey churn in nodes and edge endpoints", () => {
		const before = categoryGraph();
		const replacements = new Map(
			before.nodes.map((node) => [node.key, `${node.key}/next-tick`]),
		);
		const after: Graph = {
			...before,
			nodes: before.nodes.map((node) => ({
				...node,
				key: replacements.get(node.key) ?? node.key,
			})),
			edges: before.edges.map((edge) => ({
				...edge,
				from: replacements.get(edge.from) ?? edge.from,
				to: replacements.get(edge.to) ?? edge.to,
			})),
		};

		expect(graphVisualKey(after)).toBe(graphVisualKey(before));
	});
});

describe("hitTest", () => {
	it("resolves a node under the pointer", () => {
		const graph = categoryGraph();
		const scene = buildScene(graph, 640, 420);
		const target = scene.positions.get("category/loaded_imbalance");

		expect(target).toBeDefined();

		const hit = hitTest(graph, scene, target?.x ?? 0, target?.y ?? 0);

		expect(hit?.kind).toBe("node");

		if (hit?.kind === "node") {
			expect(hit.node.key).toBe("category/loaded_imbalance");
		}
	});

	it("resolves the nearest edge when no node is under the pointer", () => {
		// Hand-place two well-separated nodes so the assertion tests hitTest's
		// edge resolution, not incidental layout geometry.
		const graph: Graph = {
			symbol: "BTC/USD",
			at: "2026-07-14T12:00:00Z",
			nodes: [
				{
					key: "causal/a",
					kind: "concept",
					measurement: { source: "causal", metric: "a" },
				},
				{
					key: "causal/b",
					kind: "concept",
					measurement: { source: "causal", metric: "b" },
				},
			],
			edges: [
				{
					from: "causal/a",
					to: "causal/b",
					type: "conditions",
					at: "2026-07-14T12:00:00Z",
					observedFrom: "2026-07-14T12:00:00Z",
				},
			],
		};
		const scene: GraphScene = {
			width: 640,
			height: 420,
			nodes: new Map(graph.nodes.map((node) => [node.key, node])),
			positions: new Map([
				["causal/a", { x: 100, y: 200 }],
				["causal/b", { x: 500, y: 200 }],
			]),
		};

		const hit = hitTest(graph, scene, 300, 200);

		expect(hit?.kind).toBe("edge");

		if (hit?.kind === "edge") {
			expect(hit.edge.type).toBe("conditions");
		}
	});

	it("returns null on empty space", () => {
		const graph = categoryGraph();
		const scene = buildScene(graph, 640, 420);

		expect(hitTest(graph, scene, 5, 5)).toBeNull();
	});
});

describe("reciprocal edges", () => {
	const leadsLagsGraph = (): Graph => ({
		symbol: "ETH/USD",
		at: "2026-07-14T12:00:00Z",
		nodes: [
			{
				key: "anchor",
				kind: "measurement",
				measurement: { source: "leadlag", metric: "signed_lag_direction" },
			},
			{
				key: "follower",
				kind: "measurement",
				measurement: { source: "leadlag", metric: "signed_lag_direction" },
			},
		],
		edges: [
			{
				from: "anchor",
				to: "follower",
				type: "leads",
				at: "2026-07-14T12:00:00Z",
				observedFrom: "2026-07-14T12:00:00Z",
			},
			{
				from: "follower",
				to: "anchor",
				type: "lags",
				at: "2026-07-14T12:00:00Z",
				observedFrom: "2026-07-14T12:00:00Z",
			},
		],
	});

	it("flags a node pair carrying edges in both directions", () => {
		const pairs = reciprocalPairs(leadsLagsGraph());

		expect(pairs.has(pairKey("anchor", "follower"))).toBe(true);
		expect(pairs.size).toBe(1);
	});

	it("does not flag a single edge between a pair", () => {
		const graph = leadsLagsGraph();
		graph.edges = [graph.edges[0] as GraphEdge];

		expect(reciprocalPairs(graph).size).toBe(0);
	});

	it("bows the two directions to opposite sides of the chord", () => {
		const from = { x: 100, y: 200 };
		const to = { x: 500, y: 200 };
		const leads: GraphEdge = {
			from: "anchor",
			to: "follower",
			type: "leads",
			at: "",
			observedFrom: "",
		};
		const lags: GraphEdge = {
			from: "follower",
			to: "anchor",
			type: "lags",
			at: "",
			observedFrom: "",
		};

		// leads goes anchor->follower (from<to), lags goes follower->anchor.
		const leadsControl = edgeControlPoint(leads, from, to, true);
		const lagsControl = edgeControlPoint(lags, to, from, true);

		// Chord is horizontal, so the perpendicular offset is vertical; the two
		// controls must sit on opposite sides of y=200.
		expect(Math.sign(leadsControl.y - 200)).not.toBe(
			Math.sign(lagsControl.y - 200),
		);
		expect(leadsControl.y).not.toBe(200);
	});

	it("resolves a hover on a bent reciprocal edge", () => {
		const graph = leadsLagsGraph();
		const scene: GraphScene = {
			width: 640,
			height: 420,
			nodes: new Map(graph.nodes.map((node) => [node.key, node])),
			positions: new Map([
				["anchor", { x: 100, y: 200 }],
				["follower", { x: 500, y: 200 }],
			]),
		};
		const from = { x: 100, y: 200 };
		const to = { x: 500, y: 200 };
		const control = edgeControlPoint(
			graph.edges[0] as GraphEdge,
			from,
			to,
			true,
		);
		// The curve's apex (t=0.5) is halfway between the chord midpoint and the
		// control point, not at the control point itself.
		const apex = {
			x: 0.25 * from.x + 0.5 * control.x + 0.25 * to.x,
			y: 0.25 * from.y + 0.5 * control.y + 0.25 * to.y,
		};

		const hit = hitTest(graph, scene, apex.x, apex.y);

		expect(hit?.kind).toBe("edge");
		expect(apex.y).not.toBe(200);
	});
});
