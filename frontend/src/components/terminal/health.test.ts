import { describe, expect, it } from "vitest";
import { regimeValuesFromFrames, terminalHealthSummary } from "./health";

describe("signal insight health", () => {
	it("summarizes the displayed kernel set with prediction backed by resonance", () => {
		const summary = terminalHealthSummary(
			{
				measurements: {
					fluid: {
						values: () => [
							{
								symbol: "BTC/EUR",
								source: "fluid",
								confidence: 0.8,
								surprise: 0.4,
								status: "measured",
							},
						],
					},
					resonance: {
						values: () => [
							{
								symbol: "BTC/EUR",
								source: "resonance",
								confidence: 0.6,
								surprise: 1.5,
								status: "ambiguous",
							},
						],
					},
				},
				symbols: {
					"BTC/EUR": [
						{
							symbol: "BTC/EUR",
							source: "fluid",
							confidence: 0.8,
							surprise: 0.4,
							status: "measured",
						},
						{
							symbol: "BTC/EUR",
							source: "resonance",
							confidence: 0.6,
							surprise: 1.5,
							status: "ambiguous",
						},
					],
				},
			},
			"BTC/EUR",
			["fluid", "prediction", "regime"],
		);

		expect(summary.total).toBe(3);
		expect(summary.measured).toBe(1);
		expect(summary.label).toBe("Attention");
		expect(summary.bars.map((bar) => [bar.label, bar.count])).toEqual([
			["Healthy", 1],
			["Calib", 1],
			["Attention", 1],
		]);
	});

	it("uses an explicit regime frame before measurement fallback", () => {
		expect(
			regimeValuesFromFrames(
				{ measurements: {}, symbols: {} },
				{
					volatility: 0.1,
					trend: 0.2,
					bullish: 0.3,
					bearish: 0.4,
					choppiness: 0.5,
				},
			),
		).toEqual([0.1, 0.2, 0.3, 0.4, 0.5]);
	});
});
