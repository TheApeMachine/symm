import { bench, describe } from "vitest";
import { causalStore } from "./causal";

const frame = (index: number) => ({
	source: "causal",
	symbol: "BTC/USD",
	at: `2026-07-12T00:00:${String(index).padStart(2, "0")}Z`,
	samples: index,
	ready: true,
	hypothesis: "touch_support_affects_next_l3_epoch_mid_return",
	reading: {
		strength: index / 100,
		entryBaseline: 0.3,
		association: 0.1,
		intervention: 0.2,
		confidence: 0.4,
	},
});

describe("causalStore", () => {
	bench("updateFrame", () => {
		causalStore.setState(() => ({
			causal: {},
			version: 0,
		}));

		for (let index = 0; index < 130; index += 1) {
			causalStore.actions.updateFrame(frame(index));
		}
	});
});
