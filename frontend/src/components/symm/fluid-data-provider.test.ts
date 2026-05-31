import { describe, expect, it } from "vitest";

import { createFluidDataProvider } from "#/components/symm/fluid-data-provider";

describe("FluidDataProvider", () => {
	it("accumulates field_row updates into a grid snapshot", () => {
		const provider = createFluidDataProvider();
		const snapshots: unknown[] = [];

		const unregister = provider.registerSink((snapshot) => {
			snapshots.push(snapshot);
		});

		provider.ingest({
			event: "field_row",
			ts: "2026-05-31T21:00:00Z",
			symbol: "BTC/EUR",
			row: {
				symbol: "BTC/EUR",
				change_pct: 1.2,
				vol: 0.4,
				div: 0.1,
				vort: 0.2,
				turb: 0.3,
				visc: 0.05,
				re: 1200,
			},
		});

		unregister();

		expect(snapshots).toHaveLength(1);
		expect(snapshots[0]).toMatchObject({
			event: "field_snapshot",
			symbol_count: 1,
		});
		expect(
			(snapshots[0] as { grid?: { heights?: number[][] } }).grid?.heights
				?.length,
		).toBeGreaterThan(0);
	});
});
