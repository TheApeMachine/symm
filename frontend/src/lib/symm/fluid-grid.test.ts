import { describe, expect, it } from "vitest";

import { gridFromPayload } from "#/lib/symm/fluid-grid";

describe("gridFromPayload", () => {
	it("replaces null and non-finite heights with the grid minimum", () => {
		const grid = gridFromPayload({
			size: 2,
			heights: [
				[1, null as unknown as number],
				[Number.NaN, 2],
			],
			min: 0.5,
			max: 2,
			filled_cells: 3,
			outliers: {
				clipped_count: 0,
				clipped_at: 1,
				raw_max: 2,
				display_max: 1,
			},
		});

		expect(grid.heights[0][1]).toBe(0.5);
		expect(grid.heights[1][0]).toBe(0.5);
		expect(grid.heights[0][0]).toBe(1);
	});
});
