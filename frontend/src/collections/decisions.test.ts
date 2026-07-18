import { describe, expect, it } from "vitest";
import {
	DECISION_HISTORY_LIMIT,
	decisionStore,
	decisionSymbols,
	latestStrategyDecisions,
} from "./decisions";

const sampleDecision = (symbol: string, action: string, at: string) => ({
	action,
	symbol,
	at,
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
	opportunityMargin: 0.01,
	cognitiveLead: 0.2,
	basinConfidence: 0.6,
	availableCapital: 1000,
	openPositions: 1,
	slotCapacity: 4,
	cause: "edge_clear",
	reason: "utility exceeds hold",
});

describe("decisionStore", () => {
	it("retains per-symbol circular history and exposes latest snapshots", () => {
		decisionStore.actions.reset();
		decisionStore.actions.updateFrame([
			sampleDecision("BTC/USD", "enter", "2026-07-14T12:00:00Z"),
		]);
		decisionStore.actions.updateFrame([
			sampleDecision("BTC/USD", "hold", "2026-07-14T12:00:01Z"),
			sampleDecision("ETH/USD", "exit", "2026-07-14T12:00:01Z"),
		]);

		expect(decisionSymbols(decisionStore.state.decisions)).toEqual([
			"BTC/USD",
			"ETH/USD",
		]);
		expect(decisionStore.state.decisions["BTC/USD"]?.values()).toHaveLength(2);
		expect(latestStrategyDecisions(decisionStore.state.decisions)).toEqual([
			expect.objectContaining({ symbol: "BTC/USD", action: "hold" }),
			expect.objectContaining({ symbol: "ETH/USD", action: "exit" }),
		]);
	});

	it("caps per-symbol history at DECISION_HISTORY_LIMIT", () => {
		decisionStore.actions.reset();

		for (let index = 0; index < 60; index += 1) {
			decisionStore.actions.updateFrame([
				sampleDecision(
					"BTC/USD",
					"hold",
					`2026-07-14T12:00:${String(index).padStart(2, "0")}Z`,
				),
			]);
		}

		expect(decisionStore.state.decisions["BTC/USD"]?.values()).toHaveLength(
			DECISION_HISTORY_LIMIT,
		);
	});

	it("normalizes krakenfx decimal wire strings into finite numbers", () => {
		decisionStore.actions.reset();
		decisionStore.actions.updateFrame([
			{
				...sampleDecision("BTC/USD", "enter", "2026-07-14T12:00:00Z"),
				proposedNotional: "100.50",
				proposedQuantity: "0.01",
				referencePrice: "61000.25",
				expectedReturn: "0.01",
				expectedFees: "0.0002",
				expectedSpread: "0.0001",
				expectedImpact: "0.0003",
				availableCapital: "1000.00",
			},
		]);

		const decision = decisionStore.state.decisions["BTC/USD"]?.values().at(-1);

		expect(decision).toEqual(
			expect.objectContaining({
				proposedNotional: 100.5,
				proposedQuantity: 0.01,
				referencePrice: 61000.25,
				expectedReturn: 0.01,
				expectedFees: 0.0002,
				expectedSpread: 0.0001,
				expectedImpact: 0.0003,
				availableCapital: 1000,
			}),
		);
		expect(Number.isFinite(decision?.availableCapital)).toBe(true);
	});
});
