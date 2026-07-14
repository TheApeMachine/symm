import { describe, expect, it } from "vitest";
import type { GraphFrame } from "#/types/thesis";
import {
	categoryRowKey,
	forecastRowKey,
	mergeGraphFrames,
	mergeSnapshotArray,
	retainGraphFrame,
	retainLifecycleMap,
} from "./snapshot-retain";

describe("mergeSnapshotArray", () => {
	it("keeps previous rows when incoming is empty", () => {
		const previous = [
			{
				symbol: "BTC/USD",
				source: "manifold",
				target: "return",
				sourceEpoch: 1,
				at: "2026-07-14T12:00:00Z",
			},
		];

		expect(mergeSnapshotArray([], previous, forecastRowKey)).toBe(previous);
	});

	it("merges partial snapshots without dropping retained rows", () => {
		const previous = [
			{
				symbol: "BTC/USD",
				source: "manifold",
				target: "return",
				sourceEpoch: 1,
				at: "2026-07-14T12:00:00Z",
			},
		];
		const incoming = [
			{
				symbol: "ETH/USD",
				source: "causal",
				target: "return",
				sourceEpoch: 2,
				at: "2026-07-14T12:01:00Z",
			},
		];

		expect(
			mergeSnapshotArray(incoming, previous, forecastRowKey).map(
				(row) => row.symbol,
			),
		).toEqual(["BTC/USD", "ETH/USD"]);
	});
});

describe("retainGraphFrame", () => {
	const previous: GraphFrame = {
		symbol: "BTC/USD",
		at: "2026-07-14T12:00:00Z",
		nodes: [{ key: "node-a", measurement: { source: "fluid" } }],
		edges: [],
	};

	it("keeps the previous graph when the incoming frame has no nodes", () => {
		expect(
			retainGraphFrame(
				{
					symbol: "BTC/USD",
					at: "2026-07-14T12:01:00Z",
					nodes: [],
					edges: [],
				},
				previous,
			),
		).toBe(previous);
	});

	it("keeps the previous graph when the incoming frame regresses in node count", () => {
		expect(
			retainGraphFrame(
				{
					symbol: "BTC/USD",
					at: "2026-07-14T12:02:00Z",
					nodes: [
						{ key: "node-a", measurement: { source: "fluid" } },
						{ key: "node-b", measurement: { source: "hawkes" } },
					],
					edges: [],
				},
				{
					...previous,
					nodes: [
						{ key: "node-a", measurement: { source: "fluid" } },
						{ key: "node-b", measurement: { source: "hawkes" } },
						{ key: "node-c", measurement: { source: "depthflow" } },
					],
				},
			).nodes,
		).toHaveLength(3);
	});
});

describe("mergeGraphFrames", () => {
	it("does not replace a populated graph with an empty node frame", () => {
		const merged = mergeGraphFrames(
			[
				{
					symbol: "BTC/USD",
					at: "2026-07-14T12:01:00Z",
					nodes: [],
					edges: [],
				},
			],
			{
				"BTC/USD": {
					symbol: "BTC/USD",
					at: "2026-07-14T12:00:00Z",
					nodes: [{ key: "node-a", measurement: { source: "fluid" } }],
					edges: [],
				},
			},
		);

		expect(merged["BTC/USD"]?.nodes).toHaveLength(1);
	});
});

describe("retainLifecycleMap", () => {
	it("ignores blank lifecycle states while merging concrete updates", () => {
		expect(
			retainLifecycleMap(
				{ "BTC/USD": "", "ETH/USD": "shaped" },
				{ "BTC/USD": "managing" },
			),
		).toEqual({
			"BTC/USD": "managing",
			"ETH/USD": "shaped",
		});
	});
});

describe("categoryRowKey", () => {
	it("dedupes categories from store and measurement sources", () => {
		const merged = mergeSnapshotArray(
			[
				{ symbol: "BTC/USD", type: "laminar", confidence: 0.8 },
				{ symbol: "BTC/USD", type: "laminar", confidence: 0.9 },
			],
			[],
			categoryRowKey,
		);

		expect(merged).toHaveLength(1);
		expect(merged[0]?.confidence).toBe(0.9);
	});
});
