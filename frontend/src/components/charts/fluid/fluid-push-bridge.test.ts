import { describe, expect, it } from "vitest";

import {
	attachFluidPush,
	deliverFluidWire,
	parseFluidWire,
} from "#/components/charts/fluid/fluid-push-bridge";

const fluidFrame = (symbolCount: number) => ({
	type: "fluid" as const,
	ts: "2026-06-06T22:00:00Z",
	symbol_count: symbolCount,
	symbols: [
		{
			symbol: "BTC/EUR",
			change_pct: 1,
			vol: 100,
			div: 0.1,
			vort: 0.2,
			turb: 0.05,
			visc: 0.9,
			re: 0.2,
		},
	],
});

describe("parseFluidWire", () => {
	it("accepts fluid field snapshots", () => {
		expect(parseFluidWire(fluidFrame(1))).toEqual(fluidFrame(1));
	});

	it("rejects non-fluid frames", () => {
		expect(parseFluidWire({ chart: "gauge", source: "fluid" })).toBeNull();
		expect(parseFluidWire({ type: "fluid", symbols: [] })).toBeNull();
	});
});

describe("deliverFluidWire", () => {
	it("buffers only the latest snapshot until the chart is ready", () => {
		const bridge = {
			push: () => {},
			ready: false,
			pending: null,
		};
		const applied: unknown[] = [];

		deliverFluidWire(bridge, fluidFrame(1));
		deliverFluidWire(bridge, {
			...fluidFrame(2),
			ts: "2026-06-06T22:00:01Z",
		});

		expect(bridge.pending?.symbol_count).toBe(2);

		attachFluidPush(bridge, (frame) => {
			applied.push(frame);
		});

		expect(applied).toHaveLength(1);
		expect(applied[0]).toEqual({
			...fluidFrame(2),
			ts: "2026-06-06T22:00:01Z",
		});
		expect(bridge.pending).toBeNull();
	});
});
