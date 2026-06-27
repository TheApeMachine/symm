import { describe, expect, it } from "vitest";

import { cognitiveStore } from "#/collections/cognitive";

describe("cognitiveStore", () => {
	it("merges partial per-symbol reading frames", () => {
		cognitiveStore.setState(() => ({
			readings: {},
			selectedScope: null,
		}));

		cognitiveStore.actions.updateFrame({
			readings: {
				"BTC/USD": {
					scope: "BTC/USD",
					classConfidence: 0.5,
				},
			},
		});
		cognitiveStore.actions.updateFrame({
			readings: {
				"ETH/USD": {
					scope: "ETH/USD",
					classConfidence: 0.8,
				},
			},
		});

		expect(Object.keys(cognitiveStore.state.readings).sort()).toEqual([
			"BTC/USD",
			"ETH/USD",
		]);
		expect(cognitiveStore.state.readings["ETH/USD"]?.classConfidence).toBe(
			0.8,
		);
	});
});
