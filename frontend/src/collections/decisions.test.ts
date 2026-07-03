import { describe, expect, it } from "vitest";
import { decisionStore } from "#/collections/decisions";

describe("decisionStore", () => {
	it("appends raw action artifacts without building an aggregate shape", () => {
		decisionStore.actions.reset();

		decisionStore.actions.updateFrame({
			role: "buy",
			seq: 1,
			scope: "BTC/USD",
			symbol: "BTC/USD",
			side: "buy",
			verdict: "allow",
			entry_confidence: 0.7,
		});
		decisionStore.actions.updateFrame({
			role: "sell",
			seq: 2,
			scope: "ETH/USD",
			symbol: "ETH/USD",
			side: "sell",
			verdict: "deny",
		});

		expect(decisionStore.state.decisions.values()).toEqual([
			{
				role: "buy",
				seq: 1,
				scope: "BTC/USD",
				symbol: "BTC/USD",
				side: "buy",
				verdict: "allow",
				entry_confidence: 0.7,
			},
			{
				role: "sell",
				seq: 2,
				scope: "ETH/USD",
				symbol: "ETH/USD",
				side: "sell",
				verdict: "deny",
			},
		]);
		expect(decisionStore.state.allowed).toHaveLength(1);
		expect(decisionStore.state.denied).toHaveLength(1);
	});
});
