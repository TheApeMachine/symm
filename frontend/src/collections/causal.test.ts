import { beforeEach, describe, expect, it } from "vitest";
import { causalStore } from "./causal";

describe("causalStore", () => {
	beforeEach(() => {
		causalStore.setState(() => ({
			causal: {},
			version: 0,
		}));
	});

	it("accepts backend causal outcomes published on each thesis tick", () => {
		causalStore.actions.updateFrame({
			source: "causal",
			symbol: "CAUSAL/TEST",
			at: "2026-07-12T00:00:00Z",
			ready: false,
			hypothesis: "touch_support_affects_next_l3_epoch_mid_return",
			reading: {
				strength: 0.42,
				entryBaseline: 0.31,
				association: 0.12,
			},
		});

		expect(
			causalStore.state.causal["CAUSAL/TEST"]?.values().at(-1),
		).toMatchObject({
			source: "causal",
			ready: false,
			reading: { strength: 0.42, entryBaseline: 0.31 },
		});
		expect(causalStore.state.version).toBe(1);
	});

	it("replaces the superseded frame for the same symbol", () => {
		const frame = (samples: number) => ({
			source: "causal",
			symbol: "BTC/USD",
			at: "2026-07-12T00:00:00Z",
			samples,
			reading: {
				strength: samples / 10,
				entryBaseline: 0.3,
			},
		});

		causalStore.actions.updateFrame(frame(1));
		causalStore.actions.updateFrame(frame(2));

		expect(causalStore.state.causal["BTC/USD"]?.values()).toEqual([frame(2)]);
	});
});
