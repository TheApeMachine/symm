import { describe, expect, it } from "vitest";
import { liveFocusSymbol } from "./measurement-sources";

describe("liveFocusSymbol", () => {
	it("keeps the preferred major even when other symbols are live", () => {
		const state = {
			measurements: {
				"AAVE/USD": { hawkes: { length: 1 } },
				"ETH/USD": { hawkes: { length: 1 } },
			},
		} as never;

		expect(liveFocusSymbol(state, "BTC/USD")).toBe("BTC/USD");
	});
});
