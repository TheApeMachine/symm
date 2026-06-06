import { bench, describe } from "vitest";

import { parsePredictionWire } from "#/components/charts/prediction/prediction-wire";

describe("parsePredictionWire", () => {
	bench("reads a prediction chart frame", () => {
		parsePredictionWire({
			chart: "prediction",
			symbol: "BTC/EUR",
			kind: "prediction",
			x: 1_800_000_060,
			value: 0.42,
		});
	});
});
