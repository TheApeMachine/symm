
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

	it("indexes measurements by origin and symbol", () => {
		measurementsStore.actions.updateFrame({
			origin: "pumpdump",
			scope: "BTC/USD",
			output: { confidence: 0.71 },
		});
		measurementsStore.actions.updateFrame({
			origin: "toxicity",
			scope: "ETH/USD",
			output: { confidence: 0.42 },
		});
		measurementsStore.actions.updateFrame({
			origin: "pumpdump",
			scope: "NEAR/EUR",
			output: { confidence: 0.55 },
		});

		expect(measurementsStore.state.measurements.pumpdump.values()).toEqual([
			{
				origin: "pumpdump",
				scope: "BTC/USD",
				output: { confidence: 0.71 },
			},
			{
				origin: "pumpdump",
				scope: "NEAR/EUR",
				output: { confidence: 0.55 },
			},
		]);
		expect(measurementsStore.state.measurements.toxicity.values()).toEqual([
			{
				origin: "toxicity",
				scope: "ETH/USD",
				output: { confidence: 0.42 },
			},
		]);
		expect(measurementsStore.state.symbols["BTC/USD"]?.at(-1)?.output).toEqual({
			confidence: 0.71,
		});
		expect(measurementsStore.state.symbols["ETH/USD"]?.at(-1)?.output).toEqual({
			confidence: 0.42,
		});
	});

	it("keeps bounded origin and symbol histories", () => {
		for (let index = 0; index < 55; index += 1) {
			measurementsStore.actions.updateFrame({
				origin: "cvd",
				scope: "SOL/USD",
				observed_at: index,
				output: { confidence: index / 100 },
			});
		}

		const originHistory = measurementsStore.state.measurements.cvd.values();
		const symbolHistory = measurementsStore.state.symbols["SOL/USD"] ?? [];

		expect(originHistory).toHaveLength(50);
		expect(symbolHistory).toHaveLength(50);
		expect(originHistory[0]?.observed_at).toBe(5);
		expect(symbolHistory[0]?.observed_at).toBe(5);
		expect(originHistory.at(-1)?.observed_at).toBe(54);
		expect(symbolHistory.at(-1)?.observed_at).toBe(54);
	});

	it("keeps the measurements map stable while updating the touched origin", () => {
		const before = measurementsStore.state.measurements;
		const toxicity = measurementsStore.state.measurements.toxicity.values();

		measurementsStore.actions.updateFrame({
			origin: "pumpdump",
			scope: "BTC/USD",
			output: { confidence: 0.33 },
		});

		expect(measurementsStore.state.measurements).toBe(before);
		expect(measurementsStore.state.measurements.pumpdump.values().at(-1)).toEqual({
			origin: "pumpdump",
			scope: "BTC/USD",
			output: { confidence: 0.33 },
		});
		expect(measurementsStore.state.measurements.toxicity.values()).toBe(toxicity);
	});

	it("creates origin histories dynamically", () => {
		measurementsStore.actions.updateFrame({
			origin: "newkernel",
			scope: "BTC/USD",
			output: { confidence: 0.9 },
		});

		expect(measurementsStore.state.measurements.newkernel.values()).toEqual([
			{
				origin: "newkernel",
				scope: "BTC/USD",
				output: { confidence: 0.9 },
			},
		]);
	});

	it("uses symbol when scope is absent", () => {
		measurementsStore.actions.updateFrame({
			origin: "fluid",
			symbol: "BTC/USD",
			output: { confidence: 0.2 },
		});

		expect(measurementsStore.state.symbols["BTC/USD"]?.at(-1)?.output).toEqual({
			confidence: 0.2,
		});
	});
});

