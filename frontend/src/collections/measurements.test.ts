
import { beforeEach, describe, expect, it } from "vitest";
import { measurementOrigins, measurementsStore } from "#/collections/measurements";

const resetMeasurements = () =>
	measurementsStore.setState(() => ({
		measurements: measurementOrigins(),
		symbols: {},
	}));

describe("measurementsStore", () => {
	beforeEach(() => {
		resetMeasurements();
	});

	it("indexes measurements by source and symbol", () => {
		measurementsStore.actions.updateFrame({
			source: "pumpdump",
			symbol: "BTC/USD",
			confidence: 0.71,
		});
		measurementsStore.actions.updateFrame({
			source: "toxicity",
			symbol: "ETH/USD",
			confidence: 0.42,
		});
		measurementsStore.actions.updateFrame({
			source: "pumpdump",
			symbol: "NEAR/EUR",
			confidence: 0.55,
		});

		expect(measurementsStore.state.measurements.pumpdump.values()).toEqual([
			{
				source: "pumpdump",
				symbol: "BTC/USD",
				confidence: 0.71,
			},
			{
				source: "pumpdump",
				symbol: "NEAR/EUR",
				confidence: 0.55,
			},
		]);
		expect(measurementsStore.state.measurements.toxicity.values()).toEqual([
			{
				source: "toxicity",
				symbol: "ETH/USD",
				confidence: 0.42,
			},
		]);
		expect(measurementsStore.state.symbols["BTC/USD"]?.at(-1)?.confidence).toBe(0.71);
		expect(measurementsStore.state.symbols["ETH/USD"]?.at(-1)?.confidence).toBe(0.42);
	});

	it("keeps bounded source and symbol histories", () => {
		for (let index = 0; index < 55; index += 1) {
			measurementsStore.actions.updateFrame({
				source: "cvd",
				symbol: "SOL/USD",
				at: index,
				confidence: index / 100,
			});
		}

		const originHistory = measurementsStore.state.measurements.cvd.values();
		const symbolHistory = measurementsStore.state.symbols["SOL/USD"] ?? [];

		expect(originHistory).toHaveLength(50);
		expect(symbolHistory).toHaveLength(50);
		expect(originHistory[0]?.at).toBe(5);
		expect(symbolHistory[0]?.at).toBe(5);
		expect(originHistory.at(-1)?.at).toBe(54);
		expect(symbolHistory.at(-1)?.at).toBe(54);
	});

	it("keeps the measurements map stable while updating the touched origin", () => {
		const before = measurementsStore.state.measurements;
		const toxicity = measurementsStore.state.measurements.toxicity.values();

		measurementsStore.actions.updateFrame({
			source: "pumpdump",
			symbol: "BTC/USD",
			confidence: 0.33,
		});

		expect(measurementsStore.state.measurements).toBe(before);
		expect(measurementsStore.state.measurements.pumpdump.values().at(-1)).toEqual({
			source: "pumpdump",
			symbol: "BTC/USD",
			confidence: 0.33,
		});
		expect(measurementsStore.state.measurements.toxicity.values()).toBe(toxicity);
	});

	it("creates origin histories dynamically", () => {
		measurementsStore.actions.updateFrame({
			source: "newkernel",
			symbol: "BTC/USD",
			confidence: 0.9,
		});

		expect(measurementsStore.state.measurements.newkernel.values()).toEqual([
			{
				source: "newkernel",
				symbol: "BTC/USD",
				confidence: 0.9,
			},
		]);
	});

	it("uses symbol when scope is absent", () => {
		measurementsStore.actions.updateFrame({
			source: "fluid",
			symbol: "BTC/USD",
			confidence: 0.2,
		});

		expect(measurementsStore.state.symbols["BTC/USD"]?.at(-1)?.confidence).toBe(0.2);
	});
});
