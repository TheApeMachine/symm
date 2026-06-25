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

		expect(
			measurementsStore.state.readings.pumpdump?.["BTC/USD"]?.output,
		).toEqual({ confidence: 0.71 });
		expect(
			measurementsStore.state.readings.toxicity?.["ETH/USD"]?.output,
		).toEqual({ confidence: 0.42 });
		expect(
			measurementsStore.state.readings.pumpdump?.["NEAR/EUR"]?.output,
		).toEqual({ confidence: 0.55 });
	});

	it("replaces the prior reading for the same origin and scope", () => {
		measurementsStore.actions.updateReading({
			origin: "cvd",
			scope: "SOL/USD",
			output: { confidence: 0.1 },
		});
		measurementsStore.actions.updateReading({
			origin: "cvd",
			scope: "SOL/USD",
			output: { confidence: 0.9 },
		});

		expect(measurementsStore.state.readings.cvd?.["SOL/USD"]?.output).toEqual({
			confidence: 0.9,
		});
	});

});
