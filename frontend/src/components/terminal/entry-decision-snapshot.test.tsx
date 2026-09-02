import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { FrozenEntryDecision } from "./entry-decision-model";
import { EntryDecisionSnapshotView } from "./entry-decision-snapshot";

const decision: FrozenEntryDecision = {
	id: "decision-12345678",
	action: "enter",
	symbol: "SHAPE/USD",
	atNs: 1_788_000_000_000_000_000n,
	cause: "coordinated lift",
	reason: "the expected move cleared the complete trading cost",
	opportunity: true,
	opportunityType: "lift",
	opportunityPhase: "expansion",
	predictiveReady: true,
	predictiveStatus: "calibrated",
	confidence: 0.73,
	direction: 0.08,
	forecastSource: "predictive coder",
	forecastModel: "student-t",
	forecastHorizon: 12n,
	calibrationCount: 87n,
	allocationClass: "measured",
	proposedNotional: "24.50",
	proposedQuantity: "50000",
	referencePrice: "0.00049",
	availableCapital: "200.00",
	openPositions: 0n,
	entryCost: {
		entryPrice: "0.00049",
		bestAsk: "0.00049",
		bestBid: "0.00048",
		midpoint: "0.000485",
		grossNotional: "24.50",
		entryFee: "0.06",
		roundTripFees: "0.12",
		spread: "0.00001",
		impact: "0.000002",
		breakEven: "0.000493",
	},
	risk: {
		present: true,
		entryNoiseBand: "0.000004",
		riskDistance: "0.000009",
		trailDistance: "0.000007",
		minEdge: "0.000003",
		maxLoss: "1.25",
	},
	stopFloor: "0.000481",
	evidence: [
		{ key: "probability:profitable", value: 0.73 },
		{ key: "return:expected_log", value: 0.018 },
		{ key: "return:break_even_log", value: 0.005 },
	],
};

describe("EntryDecisionSnapshotView", () => {
	it("explains the immutable entry path in plain language", () => {
		const markup = renderToStaticMarkup(
			<EntryDecisionSnapshotView decision={decision} />,
		);

		expect(markup).toContain("Why SYMM entered");
		expect(markup).toContain("FROZEN AT ENTRY");
		expect(markup).toContain("Opportunity appeared");
		expect(markup).toContain("Expected move cleared costs");
		expect(markup).toContain("How convinced was the system?");
		expect(markup).toContain("model&#x27;s estimate at entry, not a promise");
		expect(markup).toContain("Estimated chance that the move clears");
		expect(markup).not.toContain("MCTS");
		expect(markup).not.toContain("causal graph");
	});
});
