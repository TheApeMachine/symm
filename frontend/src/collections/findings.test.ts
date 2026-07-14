import { describe, expect, it } from "vitest";
import { findingsList, findingsStore } from "./findings";

const finding = (symbol: string, component: string) => ({
	symbol,
	component,
	condition: "test condition",
	evidence: ["line"],
	estimatedEffect: -0.004,
	uncertainty: 0.001,
	requiredValidation: "replay",
});

describe("findingsStore", () => {
	it("deduplicates findings while appending into a circular buffer", () => {
		findingsStore.actions.reset();
		findingsStore.actions.updateFrame([finding("BTC/USD", "forecast")]);
		findingsStore.actions.updateFrame([
			finding("BTC/USD", "forecast"),
			finding("ETH/USD", "execution"),
		]);

		expect(findingsList(findingsStore.state.findings)).toHaveLength(2);
	});

	it("evicts the oldest retained finding once the circular buffer is full", () => {
		const store = findingsStore;
		store.actions.reset();

		for (let index = 0; index < 257; index += 1) {
			store.actions.updateFrame([
				finding(`SYM${index}/USD`, `component-${index}`),
			]);
		}

		const retained = findingsList(store.state.findings);

		expect(retained).toHaveLength(256);
		expect(retained[0]?.symbol).toBe("SYM1/USD");
		expect(retained.at(-1)?.symbol).toBe("SYM256/USD");
	});
});
