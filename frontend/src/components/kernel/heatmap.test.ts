import { describe, expect, it } from "vitest";
import type { Measurement } from "#/types/measurement";
import { buildHeatmapCells, heatmapLabel } from "./heatmap";

const measurement = (
	symbol: string,
	raw: number,
	metric = "strength",
	source = "liquidity",
): Measurement => ({
	at: "2026-07-14T10:00:00Z",
	symbol,
	source,
	metric,
	raw,
	unit: "dimensionless",
	normalized: null,
	uncertainty: null,
	validity: { state: "provisional", readiness: "model" },
	scale: {
		kind: "observation_window",
		from: "2026-07-14T10:00:00Z",
		through: "2026-07-14T10:00:00Z",
	},
});

describe("heatmapLabel", () => {
	it("keeps the base asset from a pair symbol", () => {
		expect(heatmapLabel("BTC/EUR")).toBe("BTC");
	});
});

describe("buildHeatmapCells", () => {
	it("collects headline strength per symbol as normalized values", () => {
		const cells = buildHeatmapCells(
			[
				measurement("BTC/EUR", 0.82),
				measurement("ETH/EUR", 0.41),
			],
			"liquidity",
			"strength",
		);

		expect(cells).toEqual([
			{ symbol: "BTC/EUR", label: "BTC", value: 0.82 },
			{ symbol: "ETH/EUR", label: "ETH", value: 0.41 },
		]);
	});

	it("includes every symbol that has a headline reading", () => {
		const measurements = Array.from({ length: 30 }, (_, index) =>
			measurement(`SYM${index}/EUR`, index / 100),
		);

		const cells = buildHeatmapCells(measurements, "liquidity", "strength");

		expect(cells).toHaveLength(30);
		expect(cells[0]?.symbol).toBe("SYM0/EUR");
		expect(cells.at(-1)?.symbol).toBe("SYM29/EUR");
	});

	it("skips symbols without a headline reading for the selected source", () => {
		const cells = buildHeatmapCells(
			[measurement("ETH/EUR", 0.5, "strength", "fluid")],
			"liquidity",
			"strength",
		);

		expect(cells).toEqual([]);
	});
});
