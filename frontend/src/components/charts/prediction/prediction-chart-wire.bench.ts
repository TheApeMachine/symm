import { bench, describe } from "vitest";

import { isPredictionWire } from "#/components/charts/prediction/prediction-chart-wire";

describe("isPredictionWire", () => {
	bench("classifies prediction chart frames", () => {
		isPredictionWire({
			chart: "prediction",
			kind: "actual",
			x: 1_710_000_000,
			value: 0.2,
		});
	});
});
