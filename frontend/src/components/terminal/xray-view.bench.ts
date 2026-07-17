import { bench, describe } from "vitest";
import { xrayLayersFromResonance } from "#/components/terminal/xray-view";

const frame = {
	source: "resonance",
	symbol: "BTC/USD",
	at: "2026-07-12T00:00:00Z",
	surprise: 0.25,
	layers: Array.from({ length: 6 }, (_, index) => ({
		state: Array.from({ length: 16 }, (__, cell) => (cell + index) * 0.01),
		prediction: Array.from({ length: 16 }, (__, cell) => (cell - index) * 0.01),
	})),
};

describe("xray-view", () => {
	bench("xrayLayersFromResonance", () => {
		xrayLayersFromResonance(frame);
	});
});
