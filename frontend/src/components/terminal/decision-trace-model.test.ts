import { describe, expect, it } from "vitest";
import { readDecisionTrace } from "#/components/terminal/decision-trace-model";

/*
stubNode builds a flatbuffer-shaped node accessor, so the reader is tested
against the accessor protocol it consumes in production rather than a plain
object it would never see.
*/
const stubNode = (fields: Record<string, unknown>, children: unknown[] = []) => ({
	actionName: () => fields.actionName ?? "",
	depth: () => fields.depth ?? 0,
	visits: () => fields.visits ?? 0,
	effectiveVisits: () => fields.effectiveVisits ?? 0,
	meanReward: () => fields.meanReward ?? 0,
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

const stubDecision = (tree: unknown, overrides: Record<string, unknown> = {}) =>
	({
		trace: () => ({
			iterations: () => 24,
			horizon: () => 5,
			recommendedAction: () => "enter",
			expectedOutcome: () => 1.5,
			outcomeUncertainty: () => 0.2,
			identificationStatus: () => "identified",
			decisionUnavailable: () => false,
			transitionSource: () => "war-room-consensus",
			consensusDominantMove: () => "explosive_pump",
			consensusConfidence: () => 0.7,
			consensusParticipants: () => 3,
			branchesLength: () => 0,
			branches: () => null,
			vetoesLength: () => 1,
			vetoes: () => "sellers absorb every market buy",
			synergiesLength: () => 0,
			synergies: () => "",
			tree: () => tree,
			...overrides,
		}),
	}) as never;

describe("readDecisionTrace", () => {
	it("returns null when the round ran no search", () => {
		expect(readDecisionTrace({ trace: () => null } as never)).toBeNull();
	});

	it("reads the council consensus and its vetoes", () => {
		const trace = readDecisionTrace(stubDecision(stubNode({ actionName: "root" })));

		expect(trace?.council.participants).toBe(3);
		expect(trace?.council.dominantMove).toBe("explosive_pump");
		expect(trace?.council.vetoes).toHaveLength(1);
	});

	it("reads the tree recursively and preserves the selected branch", () => {
		const tree = stubNode({ actionName: "root" }, [
			stubNode({ actionName: "enter", depth: 1, visits: 23, selected: true }),
			stubNode({ actionName: "wait", depth: 1, visits: 1, pruned: true }),
		]);

		const trace = readDecisionTrace(stubDecision(tree));

		expect(trace?.tree?.children).toHaveLength(2);
		expect(trace?.tree?.children[0]?.selected).toBe(true);
		expect(trace?.tree?.children[1]?.pruned).toBe(true);
	});

	it("keeps real and counterfactual evidence separate", () => {
		const tree = stubNode({ actionName: "root" }, [
			stubNode({
				actionName: "wait",
				depth: 1,
				visits: 1,
				counterfactualMass: 3.7,
				effectiveVisits: 4.7,
			}),
		]);

		const node = readDecisionTrace(stubDecision(tree))?.tree?.children[0];

		// A branch carried mostly by counterfactuals must stay visibly
		// distinct from one that was actually rolled out.
		expect(node?.visits).toBe(1);
		expect(node?.counterfactualMass).toBeCloseTo(3.7);
	});

	it("bounds recursion so a pathological tree cannot hang the render", () => {
		let deepest: ReturnType<typeof stubNode> = stubNode({ actionName: "leaf" });

		for (let index = 0; index < 50; index += 1) {
			deepest = stubNode({ actionName: `n${index}` }, [deepest]);
		}

		let depth = 0;
		let cursor = readDecisionTrace(stubDecision(deepest))?.tree ?? null;

		while (cursor) {
			depth += 1;
			cursor = cursor.children[0] ?? null;
		}

		expect(depth).toBeLessThanOrEqual(14);
	});
});
