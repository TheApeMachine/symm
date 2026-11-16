import { describe, expect, it } from "vitest";
import { decisionStoreKey } from "./book";

describe("decisionStoreKey", () => {
	it("prefixes symbol with tier when tier is set", () => {
		expect(
			decisionStoreKey({ symbol: "BTC/USD", action: "enter", tier: "real" }),
		).toBe("real:BTC/USD");
	});

	it("uses symbol alone when tier is missing", () => {
		expect(
			decisionStoreKey({ symbol: "BTC/USD", action: "skip", gate: "oracle" }),
		).toBe("BTC/USD");
	});
});