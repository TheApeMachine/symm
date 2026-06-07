import { describe, expect, it } from "vitest";

import {
	attachFluidPush,
	ingestFluidWire,
	isFieldSnapshot,
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

describe("isFieldSnapshot", () => {
	it("accepts fluid field snapshots", () => {
		expect(isFieldSnapshot(fluidFrame(1))).toBe(true);
	});

	it("rejects non-fluid frames", () => {
		expect(isFieldSnapshot({ chart: "gauge", source: "fluid" })).toBe(false);
		expect(isFieldSnapshot({ type: "fluid", symbols: [] })).toBe(false);
	});
});

describe("ingestFluidWire", () => {
	it("buffers only the latest snapshot until the chart is ready", () => {
		const bridge = {
			push: () => {},
			ready: false,
			pending: null,
		};
		const applied: unknown[] = [];

		ingestFluidWire(bridge, fluidFrame(1));
		ingestFluidWire(bridge, {
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
