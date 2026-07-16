import { bench, describe } from "vitest";
import { decisionStore } from "./decisions";
import { findingsStore } from "./findings";
import { forecastsStore } from "./forecasts";
import { graphsStore } from "./graphs";
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

const forecast = {
	source: "manifold",
	symbol: "BTC/USD",
	at: "2026-07-14T12:00:00Z",
	sourceEpoch: 1,
	horizonEvents: 2,
	expiresEpoch: 3,
	target: "return",
	modelVersion: "v1",
	ready: true,
	calibrated: true,
	frictionReady: true,
	calibrationSamples: 20,
	incrementalMSE: 0.1,
	incrementalMSELowerBound: 0.05,
	expectedReturn: 0.01,
	referencePrice: 100,
	buyCapacity: 1,
	sellCapacity: 1,
	expectedFees: 0.001,
	expectedSpread: 0.001,
	expectedImpact: 0.001,
	expectedAdverseSelection: 0.001,
	uncertainty: 0.01,
	confidence: 0.8,
};

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
				symbol: "BTC/USD",
				component: "forecast",
				condition: "expected return overstated",
				evidence: ["BTC/USD realized below forecast"],
				estimatedEffect: -0.004,
				uncertainty: 0.001,
				requiredValidation: "replay on next cohort",
			},
		]);
		graphsStore.actions.updateFrame([
			{
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
				nodes: Array.from({ length: 12 }, (_, index) => ({
					key: `node-${index}`,
					measurement: {
						source: "hawkes",
						metric: "arrival_rate",
						symbol: "BTC/USD",
					},
				})),
				edges: Array.from({ length: 11 }, (_, index) => ({
					from: `node-${index}`,
					to: `node-${index + 1}`,
					type: "supports",
					at: "2026-07-14T12:00:00Z",
					observedFrom: "2026-07-14T11:59:00Z",
				})),
			},
		]);
	});

	bench("updates bounded forecast history across source epochs", () => {
		const sourceEpoch = forecast.sourceEpoch + 1;

		forecastsStore.actions.updateFrame([{ ...forecast, sourceEpoch }]);
		forecast.sourceEpoch = sourceEpoch;
	});
});
