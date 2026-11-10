import { describe, expect, it } from "vitest";
import { bookAtom, decisionBook } from "./book";

describe("decisionBook", () => {
	it("keeps an explicit tier", () => {
		expect(decisionBook({ symbol: "BTC/USD", action: "enter", tier: "real" })).toBe(
			"real",
		);
	});

	it("maps pre-tier gate skips onto the selected book", () => {
		bookAtom.set("real");
		expect(
			decisionBook({
				symbol: "BTC/USD",
				action: "skip",
				gate: "oracle",
			}),
		).toBe("real");

		bookAtom.set("virtual");
		expect(
			decisionBook({
				symbol: "ETH/USD",
				action: "skip",
				gate: "resonance",
			}),
		).toBe("virtual");
	});
});
