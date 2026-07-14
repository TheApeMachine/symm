import { describe, expect, it } from "vitest";
import { graphsStore } from "./graphs";

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

		expect(graphsStore.state.graphs["BTC/USD"]?.nodes).toHaveLength(1);
		expect(graphsStore.state.graphs["BTC/USD"]?.edges).toHaveLength(1);
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

		expect(graphsStore.state.graphs["BTC/USD"]?.nodes).toHaveLength(1);
		expect(graphsStore.state.graphs["BTC/USD"]?.at).toBe(
			"2026-07-14T12:00:00Z",
		);
	});
});
