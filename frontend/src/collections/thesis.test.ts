import { describe, expect, it } from "vitest";
import { decisionStore } from "./decisions";
import { findingsStore } from "./findings";
import { lifecycleStore } from "./lifecycle";
import { tradeJournalStore } from "./trade-journal";

describe("thesis frame stores", () => {
	it("accepts backend strategy decisions", () => {
		decisionStore.actions.reset();
		decisionStore.actions.updateFrame([
			{
				action: "enter",
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
				utility: 0.42,
				alternatives: { hold: 0.1, exit: -0.2 },
				allocationClass: "core",
				proposedNotional: 100,
				proposedQuantity: 0.01,
				referencePrice: 61000,
				validThroughEpoch: 3,
				forecastSource: "resonance",
				forecastModel: "online",
				forecastEpoch: 2,
				calibrationCount: 4,
				expectedReturn: 0.01,
				expectedFees: 0.0002,
				expectedSpread: 0.0001,
				expectedImpact: 0.0003,
				adverseSelection: 0.0001,
				uncertainty: 0.05,
				confidence: 0.8,
				availableCapital: 1000,
				openPositions: 1,
				slotCapacity: 4,
				cause: "edge_clear",
				reason: "utility exceeds hold",
			},
		]);

		expect(decisionStore.state.decisions).toHaveLength(1);
		expect(decisionStore.state.decisions[0]?.action).toBe("enter");
	});

	it("merges backend lifecycle maps", () => {
		lifecycleStore.actions.reset();
		lifecycleStore.actions.updateFrame({ "BTC/USD": "managing" });
		lifecycleStore.actions.updateFrame({ "ETH/USD": "shaped" });

		expect(lifecycleStore.state.lifecycle).toEqual({
			"BTC/USD": "managing",
			"ETH/USD": "shaped",
		});
	});

	it("accepts backend trade journal observations", () => {
		tradeJournalStore.actions.updateFrame([
			{
				kind: "lifecycle_transition",
				symbol: "BTC/USD",
				status: "entered",
				at: "2026-07-14T12:00:01Z",
				decision: 0,
			},
		]);

		expect(tradeJournalStore.state.observations).toHaveLength(1);
		expect(tradeJournalStore.state.observations[0]?.status).toBe("entered");
	});

	it("accepts backend postmortem findings", () => {
		findingsStore.actions.reset();
		findingsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				component: "forecast",
				condition: "expected return overstated",
				evidence: ["BTC/USD realized below forecast"],
				estimatedEffect: -0.004,
				uncertainty: 0.001,
				requiredValidation: "replay on next cohort",
			},
		]);

		expect(findingsStore.state.findings).toHaveLength(1);
		expect(findingsStore.state.findings[0]?.symbol).toBe("BTC/USD");
		expect(findingsStore.state.findings[0]?.component).toBe("forecast");
	});

	it("retains findings across frames without duplicating them", () => {
		findingsStore.actions.reset();
		findingsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				component: "forecast",
				condition: "expected return overstated",
				evidence: ["BTC/USD realized below forecast"],
				estimatedEffect: -0.004,
				uncertainty: 0.001,
				requiredValidation: "replay on next cohort",
			},
		]);
		findingsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				component: "forecast",
				condition: "expected return overstated",
				evidence: ["BTC/USD realized below forecast"],
				estimatedEffect: -0.004,
				uncertainty: 0.001,
				requiredValidation: "replay on next cohort",
			},
			{
				symbol: "ETH/USD",
				component: "execution",
				condition: "fees exceeded edge",
				evidence: ["fee-1"],
				estimatedEffect: -0.002,
				uncertainty: 0.001,
				requiredValidation: "replay on next cohort",
			},
		]);

		expect(findingsStore.state.findings).toHaveLength(2);
		expect(findingsStore.state.findings[1]?.symbol).toBe("ETH/USD");
	});
});
