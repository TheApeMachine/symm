import { describe, expect, it } from "vitest";

import {
	buildFluidGrid,
	FLUID_HEIGHT_EMA_ALPHA,
	FLUID_GRID_SIZE,
	resetFluidHeightSmoothing,
} from "#/lib/symm/fluid-grid";
import type { FluidSymbolRow } from "#/lib/symm/events";

function sampleRows(re: number): FluidSymbolRow[] {
	return Array.from({ length: 8 }, (_, index) => ({
		symbol: `SYM${index}/EUR`,
		change_pct: index * 0.5,
		vol: index + 1,
		div: 0.1,
		vort: 0.2,
		turb: 0.3,
		visc: 1,
		re: re + index * 0.01,
	}));
}

describe("buildFluidGrid", () => {
	it("should EMA-smooth heights between frames", () => {
		resetFluidHeightSmoothing();

		const first = buildFluidGrid(sampleRows(10));
		const second = buildFluidGrid(sampleRows(100));

		let delta = 0;
		for (let z = 0; z < FLUID_GRID_SIZE; z++) {
			for (let x = 0; x < FLUID_GRID_SIZE; x++) {
				const firstHeight = first.heights[z][x];
				const secondHeight = second.heights[z][x];

				if (!Number.isFinite(firstHeight) || !Number.isFinite(secondHeight)) {
					continue;
				}

				delta += Math.abs(secondHeight - firstHeight);
			}
		}

		resetFluidHeightSmoothing();
		const unsmoothed = buildFluidGrid(sampleRows(100));

		let rawDelta = 0;
		for (let z = 0; z < FLUID_GRID_SIZE; z++) {
			for (let x = 0; x < FLUID_GRID_SIZE; x++) {
				const firstHeight = first.heights[z][x];
				const rawHeight = unsmoothed.heights[z][x];

				if (!Number.isFinite(firstHeight) || !Number.isFinite(rawHeight)) {
					continue;
				}

				rawDelta += Math.abs(rawHeight - firstHeight);
			}
		}

		expect(delta).toBeLessThan(rawDelta);
		expect(FLUID_HEIGHT_EMA_ALPHA).toBeGreaterThan(0);
		expect(second.max).toBeGreaterThanOrEqual(second.min);
	});
});
