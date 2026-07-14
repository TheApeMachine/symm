import { bench, describe } from "vitest";
import { decisionStore } from "./decisions";
import { findingsStore } from "./findings";
import { lifecycleStore } from "./lifecycle";
import { tradeJournalStore } from "./trade-journal";

const decisions = Array.from({ length: 8 }, (_, index) => ({
	action: index % 2 === 0 ? "enter" : "exit",
	symbol: index % 2 === 0 ? "BTC/USD" : "ETH/USD",
	at: `2026-07-14T12:00:0${index}Z`,
	utility: 0.4 + index * 0.01,
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
}));

const observations = Array.from({ length: 24 }, (_, index) => ({
	kind: index % 3 === 0 ? "lifecycle_transition" : "execution",
	symbol: index % 2 === 0 ? "BTC/USD" : "ETH/USD",
	side: index % 2 === 0 ? "buy" : "sell",
	status: "entered",
	at: `2026-07-14T12:00:${String(index).padStart(2, "0")}Z`,
	decision: index,
}));

describe("thesis frame stores", () => {
	bench("applies strategy decisions, lifecycle, journal, and findings", () => {
		decisionStore.actions.updateFrame(decisions);
		lifecycleStore.actions.updateFrame({
			"BTC/USD": "managing",
			"ETH/USD": "shaped",
		});
		tradeJournalStore.actions.updateFrame(observations);
		findingsStore.actions.updateFrame([
			{
				component: "forecast",
				condition: "expected return overstated",
				evidence: ["BTC/USD realized below forecast"],
				estimatedEffect: -0.004,
				uncertainty: 0.001,
				requiredValidation: "replay on next cohort",
			},
		]);
	});
});
