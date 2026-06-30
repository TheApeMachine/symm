import { describe, expect, it } from "vitest";
import {
	activeSymbolFor,
	focusFrameForSymbol,
	hawkesSamplesFromFrame,
	latentPointsFromFrame,
	resonanceFrameForSymbol,
} from "#/routes/xray";

describe("xray route model", () => {
	it("does not replace a concrete terminal focus with another symbol", () => {
		expect(
			activeSymbolFor(
				"XRP/USD",
				{
					focus_symbol: "ETH/EUR",
					symbols: [{ symbol: "ETH/EUR" }, { symbol: "BTC/EUR" }],
				},
				["ETH/EUR", "BTC/EUR"],
			),
		).toBe("XRP/USD");
	});

	it("uses resonance focus only in stream mode", () => {
		expect(
			activeSymbolFor(
				"stream",
				{
					focus_symbol: "ETH/EUR",
					symbols: [{ symbol: "ETH/EUR" }, { symbol: "BTC/EUR" }],
				},
				["ETH/EUR", "BTC/EUR"],
			),
		).toBe("ETH/EUR");
	});

	it("keeps the previous concrete symbol in stream mode", () => {
		expect(
			activeSymbolFor(
				"stream",
				{
					focus_symbol: "ETH/EUR",
					symbols: [{ symbol: "ETH/EUR" }, { symbol: "BTC/EUR" }],
				},
				["ETH/EUR", "BTC/EUR"],
				"BTC/EUR",
			),
		).toBe("BTC/EUR");
	});

	it("does not return the resonance focus frame for a missing concrete symbol", () => {
		expect(
			focusFrameForSymbol(
				{
					focus: { symbol: "ETH/EUR", layers: [{ state: [1] }] },
					snapshots: [{ symbol: "BTC/EUR", layers: [{ state: [2] }] }],
				},
				"XRP/EUR",
			),
		).toBeNull();
	});

	it("keeps the latest frame that actually contains the concrete focus", () => {
		const eth = { focus: { symbol: "ETH/EUR", layers: [{ state: [1] }] } };
		const btc = { focus: { symbol: "BTC/EUR", layers: [{ state: [2] }] } };

		expect(resonanceFrameForSymbol([eth, btc], btc, "ETH/EUR")).toBe(eth);
	});

	it("uses the latest frame in stream mode", () => {
		const eth = { focus: { symbol: "ETH/EUR", layers: [{ state: [1] }] } };
		const btc = { focus: { symbol: "BTC/EUR", layers: [{ state: [2] }] } };

		expect(resonanceFrameForSymbol([eth], btc, "stream")).toBe(btc);
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
