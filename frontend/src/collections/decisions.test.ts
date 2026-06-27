import { describe, expect, it } from "vitest";
import { decisionsStore } from "#/collections/decisions";

describe("decisionsStore", () => {
	it("lets authoritative batch frames clear stale candidate rows", () => {
		decisionsStore.actions.updateFrame({
			role: "decision",
			seq: 1,
			symbol: "BTC/USD",
			verdict: "blocked",
			confidence: 0.7,
		});

		expect(decisionsStore.state.frame?.decisions).toHaveLength(1);

		decisionsStore.actions.updateFrame({
			role: "decisions",
			seq: 2,
			decisions: [],
		});

		expect(decisionsStore.state.frame?.seq).toBe(2);
		expect(decisionsStore.state.frame?.decisions).toEqual([]);
	});
});
