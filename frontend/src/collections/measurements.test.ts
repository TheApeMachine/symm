import { beforeEach, describe, expect, it } from "vitest";
import { measurementsStore } from "#/collections/measurements";

const resetMeasurements = () =>
	measurementsStore.setState(() => ({
		measurements: {},
		symbols: {},
		sources: new Set<string>(),
		tick: 0,
	}));

describe("measurementsStore", () => {
	beforeEach(() => {
		resetMeasurements();
	});

	it("indexes measurements by source and symbol", () => {
		measurementsStore.actions.updateFrame({
			source: "pumpdump",
			symbol: "BTC/USD",
			categories: [{ type: "pump", confidence: 0.71, surprisal: 0, strength: 0.5 }],
			metrics: {},
		});
		measurementsStore.actions.updateFrame({
			source: "toxicity",
			symbol: "ETH/USD",
			categories: [{ type: "toxic", confidence: 0.42, surprisal: 0, strength: 0.3 }],
			metrics: {},
		});
		measurementsStore.actions.updateFrame({
			source: "pumpdump",
			symbol: "NEAR/EUR",
			categories: [{ type: "pump", confidence: 0.55, surprisal: 0, strength: 0.4 }],
			metrics: {},
		});

		expect(measurementsStore.state.measurements.pumpdump.values()).toHaveLength(2);
		expect(measurementsStore.state.measurements.toxicity.values()).toHaveLength(1);
		expect(measurementsStore.state.symbols["BTC/USD"]?.at(-1)?.categories[0]?.confidence).toBe(
			0.71,
		);
		expect(measurementsStore.state.symbols["ETH/USD"]?.at(-1)?.categories[0]?.confidence).toBe(
			0.42,
		);
	});

	it("keeps bounded source and symbol histories", () => {
		for (let index = 0; index < 55; index += 1) {
			measurementsStore.actions.updateFrame({
				source: "cvd",
				symbol: "SOL/USD",
				at: index,
				categories: [],
				metrics: {},
			});
		}

		const sourceHistory = measurementsStore.state.measurements.cvd.values();
		const symbolHistory = measurementsStore.state.symbols["SOL/USD"] ?? [];

		expect(sourceHistory).toHaveLength(50);
		expect(symbolHistory).toHaveLength(50);
		expect(sourceHistory[0]?.at).toBe(5);
		expect(symbolHistory[0]?.at).toBe("5");
		expect(sourceHistory.at(-1)?.at).toBe(54);
		expect(symbolHistory.at(-1)?.at).toBe("54");
	});

	it("creates source histories dynamically", () => {
		measurementsStore.actions.updateFrame({
			source: "newkernel",
			symbol: "BTC/USD",
			categories: [{ type: "new", confidence: 0.9, surprisal: 0, strength: 0.8 }],
			metrics: {},
		});

		expect(measurementsStore.state.measurements.newkernel.values()).toHaveLength(1);
	});

	it("uses symbol when scope is absent", () => {
		measurementsStore.actions.updateFrame({
			source: "fluid",
			symbol: "BTC/USD",
			categories: [{ type: "laminar", confidence: 0.2, surprisal: 0, strength: 0.1 }],
			metrics: {},
		});

		expect(measurementsStore.state.symbols["BTC/USD"]?.at(-1)?.categories[0]?.confidence).toBe(
			0.2,
		);
	});
});
