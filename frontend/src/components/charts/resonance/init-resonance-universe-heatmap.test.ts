import { describe, expect, it } from "vitest";
import {
	applyUniverseHeatmapRows,
	UNIVERSE_HEATMAP_MAX_ROWS,
	UNIVERSE_HEATMAP_TIME_COLS,
} from "#/components/charts/resonance/init-resonance-universe-heatmap";

describe("applyUniverseHeatmapRows", () => {
	it("keeps a fixed buffer size while updating active rows", () => {
		const zValues = Array.from({ length: UNIVERSE_HEATMAP_MAX_ROWS }, () =>
			Array.from({ length: UNIVERSE_HEATMAP_TIME_COLS }, () => 0),
		);
		const historyBySymbol = new Map<string, number[]>();

		const activeRowCount = applyUniverseHeatmapRows(
			zValues,
			historyBySymbol,
			[
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
			],
			1,
		);

		expect(activeRowCount).toBe(2);
		expect(zValues).toHaveLength(UNIVERSE_HEATMAP_MAX_ROWS);
		expect(zValues[0]?.[UNIVERSE_HEATMAP_TIME_COLS - 1]).toBeGreaterThan(
			zValues[1]?.[UNIVERSE_HEATMAP_TIME_COLS - 1] ?? 0,
		);
		expect(zValues[2]?.[UNIVERSE_HEATMAP_TIME_COLS - 1]).toBe(0);
	});
});
