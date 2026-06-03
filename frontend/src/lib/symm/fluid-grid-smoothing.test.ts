import { describe, expect, it } from "vitest";

import {
	cellSmoothRadius,
	smoothHeightmapSpatialAdaptive,
	visualStressFromAnomaly,
} from "#/lib/symm/fluid-grid-smoothing";

describe("cellSmoothRadius", () => {
	it("shrinks smoothing radius as turbulence rises", () => {
		expect(cellSmoothRadius(3, 0)).toBe(3);
		expect(cellSmoothRadius(3, 2)).toBe(1);
		expect(cellSmoothRadius(3, 10)).toBe(0);
	});
});

describe("smoothHeightmapSpatialAdaptive", () => {
	it("preserves turbulent spikes while softening calm neighbors", () => {
		const heightmap = [
			[0, 0, 0],
			[0, 10, 0],
			[0, 0, 0],
		];
		const turbulence = [
			[0, 0, 0],
			[0, 5, 0],
			[0, 0, 0],
		];

		const smoothed = smoothHeightmapSpatialAdaptive(heightmap, turbulence, 2);

		expect(smoothed[1][1]).toBe(10);
		expect(smoothed[0][1]).toBeGreaterThan(0);
		expect(smoothed[0][1]).toBeLessThan(10);
	});
});

describe("visualStressFromAnomaly", () => {
	it("amplifies highlight and hardness from anomaly SNR", () => {
		const stressed = visualStressFromAnomaly(1, 1, [[0, 2], [0, 0]]);

		expect(stressed.highlight).toBeGreaterThan(1);
		expect(stressed.cellHardnessFactor).toBeGreaterThan(1);
	});
});
