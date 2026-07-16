import { describe, expect, it } from "vitest";
import { hypothesesStore, hypothesisValues } from "./hypotheses";

describe("hypothesesStore", () => {
	it("excludes rows missing source and keeps delimiter-safe identities", () => {
		hypothesesStore.actions.reset();
		hypothesesStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				claim: "lift",
				source: "manifold",
			},
			{
				symbol: "BTC/USD",
				claim: "lift:coil",
				source: "manifold",
			},
			{
				symbol: "BTC:USD",
				claim: "lift",
				source: "manifold",
			},
			{
				symbol: "ETH/USD",
				claim: "fade",
			},
		]);

		expect(hypothesisValues(hypothesesStore.state.hypotheses)).toHaveLength(3);
	});
});
