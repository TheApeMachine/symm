import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DecisionMCTSStage } from "#/components/terminal/decision-mcts-stage";

/*
traceStub builds the flatbuffer accessor shape the stage consumes. These tests
exist because the stage was once handed a stripped plain object through an
`as any` cast, which silently removed the trace and made the whole tree render
as "no search this round" forever.
*/
const nodeStub = (
	fields: Record<string, unknown>,
	children: unknown[] = [],
) => ({
	actionName: () => fields.actionName ?? "",
	depth: () => fields.depth ?? 0,
	visits: () => fields.visits ?? 0,
	effectiveVisits: () => fields.effectiveVisits ?? 0,
	meanReward: () => 0,
	rewardStd: () => 0,
	blendedValue: () => fields.blendedValue ?? 0,
	counterfactualReward: () => 0,
	counterfactualMass: () => fields.counterfactualMass ?? 0,
	causalExpectation: () => fields.causalExpectation ?? 0,
	causalExpectationDefined: () => fields.causalExpectationDefined ?? false,
	pruned: () => fields.pruned ?? false,
	selected: () => fields.selected ?? false,
	childrenLength: () => children.length,
	children: (index: number) => children[index],
});

const decisionWithTrace = () =>
	({
		trace: () => ({
			iterations: () => 24,
			horizon: () => 5,
			recommendedAction: () => "enter",
			expectedOutcome: () => 1.25,
			outcomeUncertainty: () => 0.2,
			identificationStatus: () => "identified",
			decisionUnavailable: () => false,
			transitionSource: () => "war-room-consensus",
			consensusDominantMove: () => "explosive_pump",
			consensusConfidence: () => 0.706,
			consensusParticipants: () => 3,
			branchesLength: () => 0,
			branches: () => null,
			vetoesLength: () => 0,
			vetoes: () => "",
			synergiesLength: () => 1,
			synergies: () => "bid wall formed immediately after the sweep",
			tree: () =>
				nodeStub({ actionName: "root", visits: 24, effectiveVisits: 24 }, [
					nodeStub({
						actionName: "enter",
						depth: 1,
						visits: 23,
						effectiveVisits: 23.2,
						selected: true,
						causalExpectation: 8.54,
						causalExpectationDefined: true,
					}),
					nodeStub({
						actionName: "wait",
						depth: 1,
						visits: 1,
						effectiveVisits: 4.7,
						counterfactualMass: 3.7,
						pruned: true,
					}),
				]),
		}),
	}) as never;

describe("DecisionMCTSStage", () => {
	it("renders the council and the search tree from a real trace", () => {
		const markup = renderToStaticMarkup(
			<DecisionMCTSStage decision={decisionWithTrace()} />,
		);

		expect(markup).toContain("3 · war room");
		expect(markup).toContain("4 · causal search");
		expect(markup).toContain("explosive_pump");
		expect(markup).toContain("enter");
		expect(markup).toContain("bid wall formed immediately after the sweep");
	});

	it("shows the rollout budget and transition source", () => {
		const markup = renderToStaticMarkup(
			<DecisionMCTSStage decision={decisionWithTrace()} />,
		);

		expect(markup).toContain("24");
		expect(markup).toContain("war-room-consensus");
	});

	it("marks a pruned branch and its counterfactual mass", () => {
		const markup = renderToStaticMarkup(
			<DecisionMCTSStage decision={decisionWithTrace()} />,
		);

		expect(markup).toContain("pruned");
		expect(markup).toContain("cf");
	});

	it("keeps stage numbering stable when no search ran", () => {
		const markup = renderToStaticMarkup(
			<DecisionMCTSStage decision={{ trace: () => null } as never} />,
		);

		// An operator scanning the chain must find the same stage in the same
		// column whether or not this round searched.
		expect(markup).toContain("3 · war room");
		expect(markup).toContain("4 · causal search");
	});
});
