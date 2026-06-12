import { bench, describe } from "vitest";

import { parseCandleFrame } from "#/components/charts/trade/init-trade-chart";

describe("trade candle frames", () => {
	const frame = {
		symbol: "LTC/USD",
		sec: 1_780_627_020,
		open: 42.1,
		high: 42.3,
		low: 42,
		close: 42.2,
		volume: 14,
	};

	bench("parse candle frame", () => {
		parseCandleFrame(frame);
	});
});
