import { describe, expect, it } from "vitest";
import { instrumentsStore } from "#/collections/instruments";

describe("instrumentsStore", () => {
	it("indexes symbols from instrument frames", () => {
		instrumentsStore.actions.updateFrame({
			data: {
				pairs: [
					{ symbol: "ETH/USD", status: "online" },
					{ symbol: "BTC/USD", status: "online" },
				],
			},
		});

		instrumentsStore.actions.updateFrame({
			symbol: "SOL/USD",
			status: "online",
		});

		expect(instrumentsStore.state.symbols).toEqual([
			"BTC/USD",
			"ETH/USD",
			"SOL/USD",
		]);
		expect(instrumentsStore.state.instruments["BTC/USD"]?.status).toBe(
			"online",
		);
	});
});
