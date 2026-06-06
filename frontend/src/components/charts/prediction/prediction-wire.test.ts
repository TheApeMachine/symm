import { describe, expect, it } from "vitest";

import {
	deliverPredictionWire,
	parsePredictionWire,
} from "#/components/charts/prediction/prediction-wire";

describe("parsePredictionWire", () => {
	it("reads a prediction chart point", () => {
		expect(
			parsePredictionWire({
				chart: "prediction",
				symbol: "BTC/EUR",
				kind: "prediction",
				x: 1_800_000_060,
				value: 0.42,
			}),
		).toEqual({
			kind: "prediction",
			x: 1_800_000_060,
			value: 0.42,
		});
	});

	it("rejects malformed prediction chart points", () => {
		expect(
			parsePredictionWire({ chart: "prediction", kind: "bad" }),
		).toBeNull();
		expect(
			parsePredictionWire({
				chart: "prediction",
				kind: "actual",
				x: Number.NaN,
				value: 0.2,
			}),
		).toBeNull();
	});
});

describe("deliverPredictionWire", () => {
	it("queues readings until the SciChart bridge is ready", () => {
		const bridge = {
			append: () => {
				throw new Error("append should not run while pending");
			},
			ready: false,
			pending: [],
		};

		deliverPredictionWire(bridge, { kind: "actual", x: 1, value: 0.2 });

		expect(bridge.pending).toEqual([{ kind: "actual", x: 1, value: 0.2 }]);
	});

	it("appends readings when the SciChart bridge is ready", () => {
		const appended: unknown[] = [];
		const bridge = {
			append: (reading: unknown) => appended.push(reading),
			ready: true,
			pending: [],
		};

		deliverPredictionWire(bridge, { kind: "error", x: 2, value: 0.1 });

		expect(appended).toEqual([{ kind: "error", x: 2, value: 0.1 }]);
		expect(bridge.pending).toEqual([]);
	});
});
