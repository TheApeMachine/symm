import { describe, expect, it } from "vitest";

import {
	ingestPredictionWire,
	isPredictionWire,
	type PredictionPoint,
	registerPredictionChart,
} from "#/components/charts/prediction/prediction-chart-wire";

describe("isPredictionWire", () => {
	it("accepts prediction chart frames", () => {
		expect(
			isPredictionWire({
				chart: "prediction",
				kind: "actual",
				x: 1_710_000_000,
				value: 0.2,
			}),
		).toBe(true);
	});

	it("preserves optional horizon metadata", () => {
		const received: PredictionPoint[] = [];
		const unregister = registerPredictionChart((point) => {
			received.push(point);
		});

		ingestPredictionWire({
			chart: "prediction",
			kind: "prediction",
			x: 1_710_000_120,
			value: 0.3,
			horizon: 60,
		});

		expect(received).toEqual([
			{
				kind: "prediction",
				x: 1_710_000_120,
				value: 0.3,
				horizon: 60,
			},
		]);
		unregister();
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
	it("routes backend prediction rows to registered chart sinks", () => {
		const received: Array<{ kind: string; x: number; value: number }> = [];
		const unregister = registerPredictionChart((reading) => {
			received.push(reading);
		});

		ingestPredictionWire({
			chart: "prediction",
			kind: "error",
			x: 2,
			value: 0.1,
		});

		expect(received).toEqual([{ kind: "error", x: 2, value: 0.1 }]);
		unregister();
	});

	it("buffers readings until a chart registers", () => {
		ingestPredictionWire({
			chart: "prediction",
			kind: "actual",
			x: 1,
			value: 0.2,
		});

		const received: Array<{ kind: string; x: number; value: number }> = [];
		const unregister = registerPredictionChart((reading) => {
			received.push(reading);
		});

		expect(received).toEqual([{ kind: "actual", x: 1, value: 0.2 }]);
		unregister();
	});
});
