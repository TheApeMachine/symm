import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import {
	GraphFrame,
	GraphFrameT,
} from "#/providers/telemetry/telemetry/graph-frame";
import { GraphMetadataT } from "#/providers/telemetry/telemetry/graph-metadata";
import { GraphNodeT } from "#/providers/telemetry/telemetry/graph-node";
import { NamedNumberT } from "#/providers/telemetry/telemetry/named-number";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";
import {
	adaptGraph,
	graphFrameFromFlatBuffer,
	graphFramePlan,
	graphStructureKey,
	type MarketGraphEdge,
	type MarketGraphFrame,
	type MarketGraphNode,
	renderedNodeIds,
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

	it("renders every displayed node with at least one incident edge", () => {
		const current = frame();
		current.nodes.gamma = { id: "gamma", value: 0.3 };
		current.nodes.delta = { id: "delta", value: 0.4 };
		current.nodes.orphan = { id: "orphan", value: 9 };
		current.edges.push({
			from: "gamma",
			to: "delta",
			relation: "supports",
			weight: 0.8,
		});

		const graph = adaptGraph(current);
		const incident = new Map<string, number>();

		for (const edge of Object.values(graph.edges)) {
			incident.set(edge.source, (incident.get(edge.source) ?? 0) + 1);
			incident.set(edge.target, (incident.get(edge.target) ?? 0) + 1);
		}

		for (const nodeId of renderedNodeIds(current)) {
			expect(incident.get(nodeId)).toBeGreaterThan(0);
		}

		expect(graph.getNodeCount()).toBe(4);
		expect(graph.getEdgeCount()).toBe(2);
		expect(graph.nodes.orphan).toBeUndefined();
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
		nodeAdded.edges.push({
			from: "gamma",
			to: "alpha",
			relation: "supports",
			weight: 0.5,
		});
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

describe("graphFrameFromFlatBuffer", () => {
	it("decodes node identity, values, and metadata from the wire frame", () => {
		const builder = new flatbuffers.Builder(1024);

		const frame = new GraphFrameT(
			BigInt(1723123456789),
			BigInt(0),
			0,
			false,
			null,
			[
				new GraphNodeT(
					"BTC/USD/cvd/signed_net_fraction_zscore//dimensionless/instantaneous/epoch=1",
					null,
					"BTC/USD",
					"",
					"cvd",
					"",
					"signed_net_fraction_zscore",
					"",
					"measurement",
					-1.5,
					0,
					false,
					0,
					false,
					1.5,
					0.9,
					0,
					"dimensionless",
					BigInt(0),
					BigInt(0),
					BigInt(1723123456789),
					new GraphMetadataT(
						[new NamedNumberT("epoch", 1)],
						[new NamedStringT("timescale", "instantaneous")],
						[],
						[],
					),
					false,
				),
			],
			[],
			null,
		);

		const offset = frame.pack(builder);
		builder.finish(offset);
		const bytes = builder.asUint8Array();
		const buffer = bytes.buffer.slice(
			bytes.byteOffset,
			bytes.byteOffset + bytes.byteLength,
		);
		const fb = GraphFrame.getRootAsGraphFrame(
			new flatbuffers.ByteBuffer(new Uint8Array(buffer)),
		);

		const decoded = graphFrameFromFlatBuffer(fb);
		const node =
			decoded?.nodes?.[
				"BTC/USD/cvd/signed_net_fraction_zscore//dimensionless/instantaneous/epoch=1"
			];

		expect(node).toBeDefined();
		expect(node?.value).toBe(-1.5);
		expect(node?.strength).toBe(1.5);
		expect(node?.confidence).toBe(0.9);
		expect(node?.at).toBe("1723123456789");
		expect(node?.metadata).toEqual({
			epoch: 1,
			timescale: "instantaneous",
		});
	});
});
