import { describe, expect, it } from "vitest";
import { decisionsStore } from "#/collections/decisions";

describe("decisionsStore", () => {
	it("appends raw action artifacts without building an aggregate shape", () => {
		decisionsStore.actions.reset();

		decisionsStore.actions.updateFrame({
			role: "buy",
			seq: 1,
			scope: "BTC/USD",
			symbol: "BTC/USD",
			side: "buy",
			entry_confidence: 0.7,
		});
		decisionsStore.actions.updateFrame({
			role: "sell",
			seq: 2,
			scope: "ETH/USD",
			symbol: "ETH/USD",
			side: "sell",
		});

		expect(decisionsStore.state.frame?.seq).toBe(2);
		expect(decisionsStore.state.frames).toEqual([
			{
				role: "buy",
				seq: 1,
				scope: "BTC/USD",
				symbol: "BTC/USD",
				side: "buy",
				entry_confidence: 0.7,
			},
			{
				role: "sell",
				seq: 2,
				scope: "ETH/USD",
				symbol: "ETH/USD",
				side: "sell",
			},
		]);
		expect(decisionsStore.state.byScope["BTC/USD"]).toHaveLength(1);
	});
});
