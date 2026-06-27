import { beforeEach, describe, expect, it } from "vitest";

import { measurementsStore } from "#/collections/measurements";

describe("measurementsStore", () => {
	beforeEach(() => {
		measurementsStore.actions.reset();
	});

	it("indexes readings by origin and scope", () => {
		measurementsStore.actions.updateReading({
			origin: "pumpdump",
			scope: "BTC/USD",
			output: { confidence: 0.71 },
		});
		measurementsStore.actions.updateReading({
			origin: "toxicity",
			scope: "ETH/USD",
			output: { confidence: 0.42 },
		});
		measurementsStore.actions.updateReading({
			origin: "pumpdump",
			scope: "NEAR/EUR",
			output: { confidence: 0.55 },
		});

		expect(measurementsStore.state.pumpdump?.["BTC/USD"]?.output).toEqual({
			confidence: 0.71,
		});
		expect(measurementsStore.state.toxicity?.["ETH/USD"]?.output).toEqual({
			confidence: 0.42,
		});
		expect(measurementsStore.state.pumpdump?.["NEAR/EUR"]?.output).toEqual({
			confidence: 0.55,
		});
	});

	it("keeps only the latest raw reading per origin and scope", () => {
		measurementsStore.actions.updateReading({
			origin: "cvd",
			scope: "SOL/USD",
			observed_at: 1,
			output: { confidence: 0.1 },
		});
		measurementsStore.actions.updateReading({
			origin: "cvd",
			scope: "SOL/USD",
			observed_at: 2,
			output: { confidence: 0.9 },
		});

		expect(measurementsStore.state.cvd?.["SOL/USD"]?.output).toEqual({
			confidence: 0.9,
		});
		expect(measurementsStore.state.cvd?.["SOL/USD"]?.observed_at).toBe(2);
	});

	it("batches readings without cloning one state update per frame", () => {
		measurementsStore.actions.updateReadings([
			{
				origin: "fluid",
				scope: "BTC/USD",
				observed_at: 1,
				output: { confidence: 0.2 },
			},
			{
				origin: "fluid",
				scope: "ETH/USD",
				observed_at: 2,
				output: { confidence: 0.3 },
			},
		]);

		expect(measurementsStore.state.fluid?.["BTC/USD"]?.output).toEqual({
			confidence: 0.2,
		});
		expect(measurementsStore.state.fluid?.["ETH/USD"]?.output).toEqual({
			confidence: 0.3,
		});
	});
});
