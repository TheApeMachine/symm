import { describe, expect, it } from "vitest";

import {
	ingestPredictionWire,
	isPredictionWire,
} from "#/components/charts/prediction/prediction-wire";

describe("isPredictionWire", () => {
	it("accepts prediction chart frames", () => {
		expect(
			isPredictionWire({
				chart: "prediction",
				symbol: "BTC/EUR",
				kind: "actual",
				x: 1_710_000_000,
				value: 0.2,
			}),
		).toBe(true);
	});

	it("rejects malformed prediction frames", () => {
		expect(isPredictionWire({ chart: "prediction", kind: "bad" })).toBe(false);
		expect(
			isPredictionWire({
				chart: "prediction",
				kind: "actual",
				x: Number.NaN,
				value: 0.2,
			}),
		).toBe(false);
	});
});

describe("ingestPredictionWire", () => {
	it("buffers readings until the chart is ready", () => {
		const bridge = {
			append: () => {},
			ready: false,
			pending: [] as Array<{ kind: string; x: number; value: number }>,
		};

		ingestPredictionWire(bridge, {
			chart: "prediction",
			kind: "error",
			x: 2,
			value: 0.1,
		});

		expect(bridge.pending).toEqual([{ kind: "error", x: 2, value: 0.1 }]);
	});
});
