import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import type { Decision } from "#/types/thesis";
import { strategyStore } from "#/providers/ws-stores";
import { DecisionChain } from "./decision-chain";

const decision: Decision = {
	id: "d1",
	action: "enter",
	symbol: "BTC/USD",
	at: new Date("2026-01-01T00:00:00Z"),
	utility: 0,
	graphScore: 0.5,
	thesisScore: 0.7,
	thesisConfidence: 0.6,
	thesisSupport: 0.4,
	thesisContradiction: 0.1,
	thesisConditions: 0.3,
	direction: 1,
	allocation_haircut: 0.05,
	allocation_haircut_reason: "",
	alternatives: {},
	allocationClass: "tier1",
	opportunity: true,
	reserveEligible: false,
	predictiveReady: true,
	predictiveStatus: "ready",
	taskSkill: 0.8,
	taskSkillReady: true,
	proposedNotional: "100",
	proposedQuantity: "10",
	referencePrice: "90",
	validThroughEpoch: 0,
	forecastSource: "",
	forecastModel: "",
	forecastEpoch: 0,
	forecastHorizon: 5,
	calibrationCount: 0,
	expectedReturn: "0",
	expectedFees: "0",
	expectedSpread: "0",
	expectedImpact: "0",
	adverseSelection: "0",
	uncertainty: 0,
	confidence: 0.6,
	causalPrecision: 0,
	opportunityMargin: 0,
	cognitiveLead: 0,
	basinConfidence: 0,
	availableCapital: "1000",
	openPositions: 1,
	slotCapacity: 4,
	cause: "coil",
	reason: "admitted",
	entryCost: {
		entryPrice: "90.5",
		breakEven: "91",
		roundTripFees: "0.2",
		spread: "0.1",
		impact: "0.05",
	},
	trace: {
		graphSupports: 0.3,
		graphContradicts: 0.1,
		graphConditions: 0.2,
		thesisBalance: 0.4,
		thesisConfidence: 0.6,
		mcts: {
			iterations: 12,
			branches: [
				{ action: "enter", visits: 8, meanReward: 0.5 },
				{ action: "hold", visits: 4, meanReward: 0.2 },
			],
			recommendedAction: "enter",
		},
	},
};

describe("DecisionChain", () => {
	beforeEach(() => {
		strategyStore.setState(() => ({ outcome: "decisions", decisions: [decision] }));
	});

	afterAll(() => {
		strategyStore.setState(() => null);
	});

	it("starts compact while retaining the full structural decision trace", () => {
		const markup = renderToStaticMarkup(<DecisionChain index={0} />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · structural thesis");
		expect(markup).toContain("2 · evidence graph");
		expect(markup).toContain("3 · graph search");
		expect(markup).toContain("4 · execution + risk");
		expect(markup).toContain('data-df="symbol"');
		expect(markup).toContain('data-df="thesisScore"');
		expect(markup).toContain('data-df="thesisConfidence"');
		expect(markup).toContain('data-df="graphScore"');
		expect(markup).toContain('data-df="action"');
		expect(markup).not.toContain("edge=");
	});
});
