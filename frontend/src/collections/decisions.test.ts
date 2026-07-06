import { describe, expect, it } from "vitest";
import { decisionStore } from "#/collections/decisions";

describe("decisionStore", () => {
	it("keeps raw decision frames for the current backend tick", () => {
		decisionStore.actions.reset();

		decisionStore.actions.updateFrame({
			id: "decision-1",
			tick: 1,
			symbol: "BTC/USD",
			side: "buy",
			verdict: "allow",
			entryConfidence: 0.7,
		});
		decisionStore.actions.updateFrame({
			id: "decision-2",
			tick: 1,
			symbol: "ETH/USD",
			side: "sell",
			verdict: "deny",
		});

		expect(decisionStore.state.decisions.values()).toEqual([
			{
				id: "decision-1",
				tick: 1,
				symbol: "BTC/USD",
				side: "buy",
				verdict: "allow",
				entryConfidence: 0.7,
			},
			{
				id: "decision-2",
				tick: 1,
				symbol: "ETH/USD",
				side: "sell",
				verdict: "deny",
			},
		]);
		expect(decisionStore.state.allowed).toHaveLength(1);
		expect(decisionStore.state.denied).toHaveLength(1);
	});

	it("keeps backend decision frames when later tick frames arrive", () => {
		decisionStore.actions.reset();

		decisionStore.actions.updateFrame({
			id: "decision-1",
			tick: 1,
			symbol: "BTC/USD",
			verdict: "allow",
		});
		decisionStore.actions.observeTick(2);

		expect(decisionStore.state.tick).toBe(2);
		expect(decisionStore.state.decisions.values()).toEqual([
			{
				id: "decision-1",
				tick: 1,
				symbol: "BTC/USD",
				verdict: "allow",
			},
		]);
		expect(decisionStore.state.allowed).toEqual([
			{
				id: "decision-1",
				tick: 1,
				symbol: "BTC/USD",
				verdict: "allow",
			},
		]);
		expect(decisionStore.state.denied).toEqual([]);
	});
});
