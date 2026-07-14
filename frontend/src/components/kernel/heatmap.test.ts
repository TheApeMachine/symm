import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { MeasurementEpoch } from "#/collections/measurements";
import type { Measurement } from "#/types/measurement";
import { buildHeatmapCells, heatmapLabel } from "./heatmap";

const measurement = (
	symbol: string,
	raw: number,
	metric = "strength",
): Measurement => ({
	at: "2026-07-14T10:00:00Z",
	symbol,
	source: "liquidity",
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

const history = (values: Measurement[]) => {
	const buffer = Circular<MeasurementEpoch>(4);
	buffer.push({ at: values[0]?.at ?? "", readings: values });

	return buffer;
};

describe("heatmapLabel", () => {
	it("keeps the base asset from a pair symbol", () => {
		expect(heatmapLabel("BTC/EUR")).toBe("BTC");
	});
});

describe("buildHeatmapCells", () => {
	it("collects headline strength per symbol as normalized values", () => {
		const cells = buildHeatmapCells(
			{
				"BTC/EUR": {
					liquidity: history([measurement("BTC/EUR", 0.82)]),
				},
				"ETH/EUR": {
					liquidity: history([measurement("ETH/EUR", 0.41)]),
				},
			},
			"liquidity",
			"strength",
		);

		expect(cells).toEqual([
			{ symbol: "BTC/EUR", label: "BTC", value: 0.82 },
			{ symbol: "ETH/EUR", label: "ETH", value: 0.41 },
		]);
	});

	it("includes every symbol that has a headline reading", () => {
		const measurements = Object.fromEntries(
			Array.from({ length: 30 }, (_, index) => [
				`SYM${index}/EUR`,
				{
					liquidity: history([measurement(`SYM${index}/EUR`, index / 100)]),
				},
			]),
		);

		const cells = buildHeatmapCells(measurements, "liquidity", "strength");

		expect(cells).toHaveLength(30);
		expect(cells[0]?.symbol).toBe("SYM0/EUR");
		expect(cells.at(-1)?.symbol).toBe("SYM29/EUR");
	});

	it("skips symbols without a headline reading for the selected source", () => {
		const cells = buildHeatmapCells(
			{
				"BTC/EUR": { liquidity: Circular<MeasurementEpoch>(1) },
				"ETH/EUR": {
					fluid: history([measurement("ETH/EUR", 0.5, "strength")]),
				},
			},
			"liquidity",
			"strength",
		);

		expect(cells).toEqual([]);
	});
});
