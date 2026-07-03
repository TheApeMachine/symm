import { describe, expect, it } from "vitest";
import { instrumentsStore } from "#/collections/instruments";

describe("instrumentsStore", () => {
	it("indexes symbols from instrument artifacts", () => {
		instrumentsStore.actions.updateFrame({
			role: "instrument",
			data: {
				pairs: [
					{ symbol: "ETH/USD", status: "online" },
					{ symbol: "BTC/USD", status: "online" },
				],
			},
		});

		instrumentsStore.actions.updateFrame({
			role: "instrument",
			scope: "SOL/USD",
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
