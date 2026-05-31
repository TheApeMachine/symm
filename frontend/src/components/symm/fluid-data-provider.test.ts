import { describe, expect, it } from "vitest";

import { createFluidDataProvider } from "#/components/symm/fluid-data-provider";
import { buildFluidGrid, FLUID_GRID_SIZE } from "#/lib/symm/fluid-grid";

describe("FluidDataProvider", () => {
	it("accumulates field_row updates into a grid snapshot", () => {
		const fieldRow = {
			symbol: "BTC/EUR",
			change_pct: 1.2,
			vol: 0.4,
			div: 0.1,
			vort: 0.2,
			turb: 0.3,
			visc: 0.05,
			re: 1200,
		};
		const expectedGrid = buildFluidGrid([fieldRow]);
		const provider = createFluidDataProvider();
		const snapshots: unknown[] = [];

		const unregister = provider.registerSink((snapshot) => {
			snapshots.push(snapshot);
		});

		provider.ingest({
			event: "field_row",
			ts: "2026-05-31T21:00:00Z",
			symbol: "BTC/EUR",
			row: fieldRow,
		});

		unregister();

		expect(snapshots).toHaveLength(1);
		expect(snapshots[0]).toMatchObject({
			event: "field_snapshot",
			symbol_count: 1,
		});

		const heights = (snapshots[0] as { grid?: { heights?: number[][] } }).grid
			?.heights;
		expect(heights?.length).toBeGreaterThan(0);
		expect(heights?.length).toBe(FLUID_GRID_SIZE);

		const center = Math.floor(FLUID_GRID_SIZE / 2);
		const centerHeight = heights?.[center]?.[center] ?? 0;
		const cornerHeight = heights?.[0]?.[0] ?? 0;
		const expectedCenter = expectedGrid.heights[center]?.[center] ?? 0;
		const expectedCorner = expectedGrid.heights[0]?.[0] ?? 0;

		expect(centerHeight).toBeCloseTo(expectedCenter, 5);
		expect(cornerHeight).toBeCloseTo(expectedCorner, 5);
		expect(centerHeight).toBeGreaterThan(cornerHeight);
		expect(centerHeight).toBeGreaterThan(0);
	});
});
