import { beforeEach, describe, expect, it } from "vitest";

import {
	buildFluidGrid,
	gridFromPayload,
	resetFluidHeightSmoothing,
	summarizeFluidScaling,
} from "#/lib/symm/fluid-grid";
import type { FluidSymbolRow } from "#/lib/symm/events";

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

describe("buildFluidGrid", () => {
	beforeEach(() => {
		resetFluidHeightSmoothing();
	});

	const rows = (mutate?: (row: FluidSymbolRow, index: number) => void) =>
		Array.from({ length: 64 }, (_, index) => {
			const row: FluidSymbolRow = {
				symbol: `SYM${index}/EUR`,
				change_pct: index * 0.1,
				vol: index + 1,
				div: 0,
				vort: 0,
				turb: 0,
				visc: 1,
				re: 0,
			};

			mutate?.(row, index);

			return row;
		});

	it("uses divergence when Reynolds is zero", () => {
		const grid = buildFluidGrid(
			rows((row, index) => {
				row.div = -(index + 1) * 0.1;
			}),
		);
		const peak = Math.max(...grid.heights.flat());

		expect(peak).toBeGreaterThan(0);
		expect(grid.outliers.rawMax).toBeGreaterThan(0);
	});

	it("preserves active height when inactive symbols share the same bin", () => {
		const flatRows = Array.from({ length: 100 }, (_, index) => ({
			symbol: `FLAT${index}/EUR`,
			change_pct: index * 0.1,
			vol: (index + 1) * 10,
			div: 0,
			vort: 0,
			turb: 0,
			visc: 1,
			re: 0,
		}));
		flatRows[99] = {
			symbol: "HOT/EUR",
			change_pct: 50,
			vol: 1e6,
			div: -12,
			vort: 0.5,
			turb: 2,
			visc: 1,
			re: 100,
		};

		const grid = buildFluidGrid(flatRows);

		expect(grid.heights[31][31]).toBeGreaterThan(0);
		expect(grid.outliers.rawMaxSymbol).toBe("HOT/EUR");
	});

	it("does not use volume as a field fallback", () => {
		const grid = buildFluidGrid(rows());
		const peak = Math.max(...grid.heights.flat());

		expect(peak).toBe(0);
	});

	it("ignores zero activity rows when deriving the clip scale", () => {
		const flatRows = rows();
		flatRows[flatRows.length - 1].div = -12;
		const summary = summarizeFluidScaling(flatRows);

		expect(summary.clippedAt).toBeGreaterThan(0);
		expect(summary.displayMax).toBeGreaterThan(0);
		expect(summary.rawMaxSymbol).toBe(flatRows[flatRows.length - 1].symbol);
	});
});
