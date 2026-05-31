import { describe, expect, it } from "vitest";

import {
	blendHeightmapTowardPeaks,
	bilinearSampleGrid,
	buildFluidGrid,
	projectFluidGridToHeightmap,
	smoothHeightmapSpatial,
	spatialSmoothRadius,
} from "#/lib/symm/fluid-grid";

describe("bilinearSampleGrid", () => {
	it("interpolates between neighboring cells", () => {
		const grid = [
			[0, 2],
			[0, 4],
		];

		expect(bilinearSampleGrid(grid, 0, 0)).toBe(0);
		expect(bilinearSampleGrid(grid, 0, 0.5)).toBe(1);
		expect(bilinearSampleGrid(grid, 0.5, 0.5)).toBe(1.5);
		expect(bilinearSampleGrid(grid, 1, 1)).toBe(4);
	});
});

describe("smoothHeightmapSpatial", () => {
	it("softens isolated spikes and lifts neighboring cells", () => {
		const heightmap = [
			[0, 0, 0],
			[0, 10, 0],
			[0, 0, 0],
		];
		const smoothed = smoothHeightmapSpatial(heightmap, 1);

		expect(smoothed[1][1]).toBeLessThan(10);
		expect(smoothed[1][1]).toBeGreaterThan(0);
		expect(smoothed[0][1]).toBeGreaterThan(0);
		expect(smoothed[1][0]).toBeGreaterThan(0);
	});
});

describe("blendHeightmapTowardPeaks", () => {
	it("restores part of the raw peak after smoothing", () => {
		const raw = [
			[0, 0],
			[0, 10],
		];
		const smoothed = [
			[0, 2],
			[2, 4],
		];
		const blended = blendHeightmapTowardPeaks(smoothed, raw, 0.5);

		expect(blended[1][1]).toBeGreaterThan(smoothed[1][1]);
		expect(blended[1][1]).toBeLessThan(raw[1][1]);
	});
});

describe("projectFluidGridToHeightmap", () => {
	it("softens spike height while keeping a visible hotspot", () => {
		const grid = {
			heights: Array.from({ length: 5 }, (_, zIndex) =>
				Array.from({ length: 5 }, (_, xIndex) =>
					zIndex === 2 && xIndex === 2 ? 10 : 1,
				),
			),
			min: 1,
			max: 10,
			filledCells: 25,
			outliers: {
				clippedCount: 0,
				clippedAt: 10,
				rawMax: 10,
				displayMax: 10,
			},
		};
		const projected = projectFluidGridToHeightmap(grid, 10, 10, -0.3, 0.3);

		expect(projected.display[5][5]).toBeLessThan(projected.raw[5][5]);
		expect(projected.display[5][5]).toBeGreaterThan(projected.display[4][5]);
	});
});

describe("buildFluidGrid", () => {
	it("keeps a visible hotspot when only one symbol is present", () => {
		const grid = buildFluidGrid([
			{
				symbol: "BTC/EUR",
				change_pct: 1.2,
				vol: 400,
				div: 0.1,
				vort: 0.2,
				turb: 0.3,
				visc: 0.05,
				re: 1200,
			},
		]);
		const projected = projectFluidGridToHeightmap(grid, 50, 50, -0.3, 0.3);
		const values = projected.display.flat();
		const min = Math.min(...values);
		const max = Math.max(...values);

		expect(max - min).toBeGreaterThan(0.05);
	});
});

describe("spatialSmoothRadius", () => {
	it("scales with grid size", () => {
		expect(spatialSmoothRadius(50, 50)).toBeGreaterThan(
			spatialSmoothRadius(16, 16),
		);
	});
});
