import { describe, expect, it } from "vitest";
import { regimeValuesFromFrames, terminalHealthSummary } from "./health";

describe("signal insight health", () => {
	it("summarizes the displayed kernel set with prediction backed by resonance", () => {
		const summary = terminalHealthSummary(
			{
				fluid: {
					"BTC/EUR": { confidence: 0.8, surprise: 0.4 },
				},
				resonance: {
					"BTC/EUR": { confidence: 0.6, surprise: 1.5 },
				},
			},
			"BTC/EUR",
			["fluid", "prediction", "regime"],
		);

		expect(summary.total).toBe(3);
		expect(summary.healthy).toBe(1);
		expect(summary.avg).toBe(47);
		expect(summary.firing).toBe(1);
		expect(summary.bars.map((bar) => [bar.label, bar.count])).toEqual([
			["Healthy", 1],
			["Warming", 2],
			["Degraded", 0],
		]);
	});

	it("uses an explicit regime frame before measurement fallback", () => {
		expect(
			regimeValuesFromFrames(
				{},
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
