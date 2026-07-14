import { bench, describe } from "vitest";
import { resonanceStore } from "./resonance";

const frame = (index: number) => ({
	source: "resonance",
	symbol: "BTC/USD",
	at: `2026-07-12T00:00:${String(index).padStart(2, "0")}Z`,
	surprise: 0.5,
	layers: [{ state: [index], prediction: [index + 0.5] }],
});

describe("resonanceStore", () => {
	bench("updateFrame", () => {
		resonanceStore.setState(() => ({
			resonance: {},
			version: 0,
		}));

		for (let index = 0; index < 130; index += 1) {
			resonanceStore.actions.updateFrame(frame(index));
		}
	});
});
