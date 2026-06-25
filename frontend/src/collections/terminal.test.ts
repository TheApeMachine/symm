import { beforeEach, describe, expect, it } from "vitest";

import { terminalStore } from "#/collections/terminal";

describe("terminalStore", () => {
	beforeEach(() => {
		terminalStore.setState(() => ({
			scanlines: true,
			fieldStyle: "Heatmap",
			selectedSource: "causal",
			inspectorSource: null,
			paletteOpen: false,
			paletteQuery: "",
			paletteIndex: 0,
			focusSymbol: "stream",
		}));
	});

	it("tracks inspector and focus symbol selection", () => {
		terminalStore.actions.inspectSource("hawkes");
		terminalStore.actions.selectFocusSymbol("BTC/USD");

		expect(terminalStore.state.inspectorSource).toBe("hawkes");
		expect(terminalStore.state.selectedSource).toBe("hawkes");
		expect(terminalStore.state.focusSymbol).toBe("BTC/USD");
	});
});
