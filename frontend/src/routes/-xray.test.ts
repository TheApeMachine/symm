import { describe, expect, it } from "vitest";
import {
	activeSymbolFor,
	hawkesSamplesFromFrame,
	latentPointsFromFrame,
} from "#/routes/xray";

describe("xray route model", () => {
	it("falls back to the resonance focus when terminal focus is stale", () => {
		expect(
			activeSymbolFor(
				"XRP/USD",
				{
					focus_symbol: "ETH/EUR",
					symbols: [{ symbol: "ETH/EUR" }, { symbol: "BTC/EUR" }],
				},
				["ETH/EUR", "BTC/EUR"],
			),
		).toBe("ETH/EUR");
	});

	it("reads Hawkes intensity history from backend measurement frames", () => {
		const samples = hawkesSamplesFromFrame(
			{
				output: { intensity: 3 },
				timestamp: 3,
				history: [
					{ output: { intensity: 1 }, timestamp: 1 },
					{ output: { intensity: 2 }, timestamp: 2 },
				],
			},
			"ETH/EUR",
		);

		expect(samples).toEqual([
			{ key: "1", symbol: "ETH/EUR", intensity: 1 },
			{ key: "2", symbol: "ETH/EUR", intensity: 2 },
			{ key: "3", symbol: "ETH/EUR", intensity: 3 },
		]);
	});

	it("projects resonance universe latent vectors into scatter points", () => {
		const points = latentPointsFromFrame({
			symbols: [
				{
					symbol: "ETH/EUR",
					category: "laminar_resonance",
					latent: [0.1, 0.2, 0.3],
				},
				{ symbol: "BROKEN/EUR", latent: [0.4] },
			],
		});

		expect(points).toEqual([
			{
				key: "ETH/EUR:0",
				symbol: "ETH/EUR",
				x: 0.1,
				y: 0.2,
				category: "laminar_resonance",
			},
		]);
	});
});
