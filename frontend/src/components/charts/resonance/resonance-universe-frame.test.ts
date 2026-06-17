import { describe, expect, it } from "vitest";
import {
	normalizedSurpriseReading,
	parseResonanceUniverseFrame,
	sortedUniverseSymbols,
} from "#/components/charts/resonance/resonance-universe-frame";

const sampleFocus = {
	type: "resonance",
	symbol: "ETH/USD",
	surprise: 0.9,
	energy: 1.2,
	confidence: 0.5,
	category: "turbulent_resonance",
	layers: [
		{
			state: [1, 2, 3, 4],
			prediction: [0.9, 1.8, 2.9, 3.8],
			error_norm: 0.05,
		},
		{
			state: [0.1, 0.2, 0.3],
			prediction: [0.08, 0.18, 0.28],
			error_norm: 0.02,
		},
	],
};

describe("parseResonanceUniverseFrame", () => {
	it("accepts batched resonance universe wire frames", () => {
		const frame = parseResonanceUniverseFrame({
			type: "resonance_universe",
			ts: "2024-01-01T00:00:00Z",
			symbol_count: 2,
			focus_symbol: "ETH/USD",
			symbols: [
				{
					symbol: "BTC/USD",
					surprise: 0.2,
					energy: 0.4,
					confidence: 0.8,
					category: "laminar_resonance",
					strength: 0.3,
					latent: [0.1, 0.2, 0.3],
				},
				{
					symbol: "ETH/USD",
					surprise: 0.9,
					energy: 1.2,
					confidence: 0.5,
					category: "turbulent_resonance",
					strength: 0.7,
					latent: [0.4, 0.5, 0.6],
				},
			],
			focus: {
				...sampleFocus,
				symbol: "ETH/USD",
			},
		});

		expect(frame?.symbolCount).toBe(2);
		expect(frame?.focusSymbol).toBe("ETH/USD");
		expect(frame?.symbols).toHaveLength(2);
		expect(frame?.focus.symbol).toBe("ETH/USD");
	});

	it("rejects frames with invalid latent vectors", () => {
		const frame = parseResonanceUniverseFrame({
			type: "resonance_universe",
			ts: "2024-01-01T00:00:00Z",
			symbol_count: 1,
			focus_symbol: "ETH/USD",
			symbols: [
				{
					symbol: "ETH/USD",
					surprise: 0.9,
					energy: 1.2,
					confidence: 0.5,
					category: "turbulent_resonance",
					strength: 0.7,
					latent: [0.4, 0.5],
				},
			],
			focus: {
				...sampleFocus,
				symbol: "ETH/USD",
			},
		});

		expect(frame).toBeNull();
	});
});

describe("sortedUniverseSymbols", () => {
	it("orders symbols by descending surprise", () => {
		const sorted = sortedUniverseSymbols([
			{
				symbol: "BTC/USD",
				surprise: 0.2,
				energy: 0,
				confidence: 0,
				category: "",
				strength: 0,
				latent: [0, 0, 0],
			},
			{
				symbol: "ETH/USD",
				surprise: 0.9,
				energy: 0,
				confidence: 0,
				category: "",
				strength: 0,
				latent: [0, 0, 0],
			},
		]);

		expect(sorted[0]?.symbol).toBe("ETH/USD");
	});
});

describe("normalizedSurpriseReading", () => {
	it("compresses heavy-tailed surprise values into zero-one range", () => {
		const low = normalizedSurpriseReading(0.2, 1000);
		const high = normalizedSurpriseReading(900, 1000);

		expect(high).toBeGreaterThan(low);
		expect(high).toBeLessThanOrEqual(1);
	});
});
