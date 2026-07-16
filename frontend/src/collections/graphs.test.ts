import { describe, expect, it } from "vitest";
import { graphsStore, latestGraphFrame } from "./graphs";

describe("graphsStore", () => {
	it("retains the latest evidence graph per symbol", () => {
		graphsStore.actions.reset();
		graphsStore.actions.updateFrame([
			{
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
			},
		]);

		const graph = latestGraphFrame(graphsStore.state.graphs, "BTC/USD");

		expect(graph?.nodes).toHaveLength(1);
		expect(graph?.edges).toHaveLength(1);
	});

	it("does not replace a populated graph with an empty node frame", () => {
		graphsStore.actions.reset();
		graphsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
				nodes: [{ key: "node-a", measurement: { source: "fluid" } }],
				edges: [],
			},
		]);
		graphsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				at: "2026-07-14T12:01:00Z",
				nodes: [],
				edges: [],
			},
		]);

		const graph = latestGraphFrame(graphsStore.state.graphs, "BTC/USD");

		expect(graph?.nodes).toHaveLength(1);
		expect(graph?.at).toBe("2026-07-14T12:00:00Z");
	});

	it("replaces an older graph when the current topology is smaller", () => {
		graphsStore.actions.reset();
		graphsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
				nodes: [
					{ key: "node-a", measurement: { source: "fluid" } },
					{ key: "node-b", measurement: { source: "hawkes" } },
				],
				edges: [],
			},
		]);
		graphsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				at: "2026-07-14T12:01:00Z",
				nodes: [{ key: "node-c", measurement: { source: "cvd" } }],
				edges: [],
			},
		]);

		const graph = latestGraphFrame(graphsStore.state.graphs, "BTC/USD");

		expect(graph?.nodes).toHaveLength(1);
		expect(graph?.at).toBe("2026-07-14T12:01:00Z");
	});
});
