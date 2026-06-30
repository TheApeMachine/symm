import { describe, expect, it } from "vitest";
import {
	collectSymbolPairs,
	isSymbolPair,
	symbolPairFromText,
	symbolPairsFromText,
	symbolsFromReadings,
} from "./symbols";

describe("terminal symbol helpers", () => {
	it("recognizes exact symbol pairs without accepting arbitrary slashed text", () => {
		expect(isSymbolPair("BTC/USD")).toBe(true);
		expect(isSymbolPair("btc/eur")).toBe(true);
		expect(isSymbolPair("signal/fluid")).toBe(false);
	});

	it("extracts one clicked symbol only when the text is unambiguous", () => {
		expect(symbolPairFromText("Position Opened: SOL/USD")).toBe("SOL/USD");
		expect(symbolPairFromText("ETH/USD vs BTC/USD")).toBeNull();
	});

	it("indexes symbols from raw artifact trees and measurement scopes", () => {
		expect(
			collectSymbolPairs({
				focus_symbol: "ETH/EUR",
				snapshots: [{ symbol: "BTC/EUR" }],
				decisions: [{ symbol: "SOL/USD" }],
			}),
		).toEqual(["BTC/EUR", "ETH/EUR", "SOL/USD"]);
		expect(
			symbolsFromReadings({
				fluid: {
					"ADA/USD": { output: { confidence: 0.4 } },
					stream: { output: { confidence: 0.1 } },
				},
			}),
		).toEqual(["ADA/USD"]);
		expect(symbolPairsFromText("mark DOGE/USD · stop DOGE/USD")).toEqual([
			"DOGE/USD",
		]);
	});
});
