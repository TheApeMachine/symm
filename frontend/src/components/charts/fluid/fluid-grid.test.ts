import { describe, expect, it } from "vitest";

import {
	buildFluidGrid,
	projectFluidGridToHeightmap,
	resetFluidHeightSmoothing,
} from "#/components/charts/fluid/fluid-grid";

const sampleRows = () => [
	{
		symbol: "BTC/EUR",
		change_pct: -2.5,
		vol: 120,
		div: 0.35,
		vort: 0.8,
		turb: 0.12,
		visc: 0.9,
		re: 0.8,
	},
	{
		symbol: "ETH/EUR",
		change_pct: 1.2,
		vol: 80,
		div: -0.2,
		vort: 0.4,
		turb: 0.05,
		visc: 0.85,
		re: 0.4,
	},
	{
		symbol: "SOL/EUR",
		change_pct: 4.8,
		vol: 40,
		div: 0.1,
		vort: 1.2,
		turb: 0.2,
		visc: 0.7,
		re: 1.2,
	},
];

describe("buildFluidGrid", () => {
	it("produces non-flat heights for active symbols", () => {
		resetFluidHeightSmoothing();

		const grid = buildFluidGrid(sampleRows());
		const values = grid.heights
			.flat()
			.filter((value) => Number.isFinite(value));

		expect(values.length).toBeGreaterThan(0);
		expect(Math.max(...values)).toBeGreaterThan(Math.min(...values));
	});

	it("projects to a heightmap with visible vertical span", () => {
		resetFluidHeightSmoothing();

		const grid = buildFluidGrid(sampleRows());
		const projected = projectFluidGridToHeightmap(grid, 32, 32, -0.3, 0.5);
		const displayValues = projected.display.flat();

		expect(Math.max(...displayValues)).toBeGreaterThan(
			Math.min(...displayValues),
		);
	});
});
