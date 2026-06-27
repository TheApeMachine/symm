import { describe, expect, it } from "vitest";

import { playbookStore } from "#/collections/playbook";

describe("playbookStore", () => {
	it("merges partial per-symbol evaluation frames", () => {
		playbookStore.setState(() => ({
			evaluations: {},
		}));

		playbookStore.actions.updateFrame({
			evaluations: {
				"BTC/USD": {
					symbol: "BTC/USD",
					steps: [{ path: [0], outcome: "matched" }],
				},
			},
		});
		playbookStore.actions.updateFrame({
			evaluations: {
				"ETH/USD": {
					symbol: "ETH/USD",
					steps: [{ path: [1], outcome: "rejected" }],
				},
			},
		});

		expect(Object.keys(playbookStore.state.evaluations).sort()).toEqual([
			"BTC/USD",
			"ETH/USD",
		]);
		expect(playbookStore.state.evaluations["ETH/USD"]?.steps[0]?.outcome).toBe(
			"rejected",
		);
	});
});
