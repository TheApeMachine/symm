import { describe, expect, it } from "vitest";
import { DEFAULT_FOCUS_SYMBOL } from "#/collections/terminal";
import { paletteSymbols } from "#/components/terminal/palette";

describe("paletteSymbols", () => {
	it("keeps BTC first and includes backend frame symbols", () => {
		const symbols = paletteSymbols({
			instruments: ["ETH/USD"],
			measurementSymbols: ["SOL/USD"],
			decisions: [{ symbol: "DOGE/USD" }],
			resonanceLatest: {
				focus_symbol: "TRX/USD",
				symbols: [{ symbol: "ADA/USD" }],
				snapshots: [{ symbol: "XRP/USD" }],
			},
			resonanceFrames: [{ focus: { symbol: "LINK/USD" } }],
		});

		expect(symbols[0]).toBe(DEFAULT_FOCUS_SYMBOL);
		expect(symbols).toEqual([
			"BTC/USD",
			"ADA/USD",
			"DOGE/USD",
			"ETH/USD",
			"LINK/USD",
			"SOL/USD",
			"TRX/USD",
			"XRP/USD",
		]);
	});
});
