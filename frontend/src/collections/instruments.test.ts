import { describe, expect, it } from "vitest";
import { appStore, DEFAULT_FOCUS_SYMBOL } from "./app";
import { instrumentsStore } from "./instruments";

describe("instrumentsStore", () => {
	it("keeps the default focus instead of jumping to the lexical first pair", () => {
		appStore.actions.updateFocusSymbol(DEFAULT_FOCUS_SYMBOL);

		instrumentsStore.actions.updateFrame([
			{ symbol: "AAVE/USD", quote: "USD", status: "online" },
			{ symbol: "ETH/USD", quote: "USD", status: "online" },
		]);

		expect(appStore.state.focusSymbol).toBe(DEFAULT_FOCUS_SYMBOL);
		expect(instrumentsStore.state.symbols).toEqual(["AAVE/USD", "ETH/USD"]);
	});

	it("indexes instrument snapshots for symbol search", () => {
		instrumentsStore.actions.updateFrame({
			data: {
				pairs: [
					{ symbol: "BTC/USD", quote: "USD", status: "online" },
					{ symbol: "SOL/USD", quote: "USD", status: "online" },
				],
			},
		});

		expect(instrumentsStore.state.symbols).toContain("BTC/USD");
		expect(instrumentsStore.state.symbols).toContain("SOL/USD");
	});
});
