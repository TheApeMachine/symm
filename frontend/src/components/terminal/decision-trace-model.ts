import type { Decision } from "#/providers/telemetry/telemetry/decision";

/*
The live decision trace, read straight off the telemetry decision. Real rollout
evidence and virtual counterfactual evidence are kept separate all the way to
the screen: a branch that won because Pearl's counterfactuals filled in its
value should not look identical to one that won by being rolled out repeatedly.
*/
export type TraceNode = {
	action: string;
	depth: number;
	visits: number;
	effectiveVisits: number;
	meanReward: number;
	rewardStd: number;
	blendedValue: number;
	counterfactualReward: number;
	counterfactualMass: number;
	causalExpectation: number;
	causalExpectationDefined: boolean;
	pruned: boolean;
	selected: boolean;
	children: TraceNode[];
};

export type TraceBranch = {
	action: string;
	visits: number;
	meanReward: number;
	blendedValue: number;
	counterfactualMass: number;
	counterfactualMean: number;
	effectiveVisits: number;
	causalExpectation: number;
	causalExpectationDefined: boolean;
	pruned: boolean;
};

export type CouncilTrace = {
	dominantMove: string;
	confidence: number;
	participants: number;
	vetoes: string[];
	synergies: string[];
};

export type DecisionTraceModel = {
	council: CouncilTrace;
	iterations: number;
	horizon: number;
	recommendedAction: string;
	expectedOutcome: number;
	outcomeUncertainty: number;
	identificationStatus: string;
	decisionUnavailable: boolean;
	transitionSource: string;
	branches: TraceBranch[];
	tree: TraceNode | null;
};

/*
readNode walks one flatbuffer subtree. Depth is bounded because a corrupted or
unexpectedly deep tree must never hang the render loop.
*/
const MAX_RENDER_DEPTH = 12;

type NodeAccessor = {
	actionName?: () => string | null;
	depth?: () => number | bigint;
	visits?: () => number | bigint;
	effectiveVisits?: () => number;
	meanReward?: () => number;
	rewardStd?: () => number;
	blendedValue?: () => number;
	counterfactualReward?: () => number;
	counterfactualMass?: () => number;
	causalExpectation?: () => number;
	causalExpectationDefined?: () => boolean;
	pruned?: () => boolean;
	selected?: () => boolean;
	childrenLength?: () => number;
	children?: (index: number) => NodeAccessor | null;
};

const readNode = (
	node: NodeAccessor | null,
	depth: number,
): TraceNode | null => {
	if (!node || depth > MAX_RENDER_DEPTH) {
		return null;
	}

	const children: TraceNode[] = [];
	const count = node.childrenLength?.() ?? 0;

	for (let index = 0; index < count; index += 1) {
		const child = readNode(node.children?.(index) ?? null, depth + 1);

		if (child) {
			children.push(child);
		}
	}

	return {
		action: node.actionName?.() ?? "",
		depth: Number(node.depth?.() ?? depth),
		visits: Number(node.visits?.() ?? 0),
		effectiveVisits: node.effectiveVisits?.() ?? 0,
		meanReward: node.meanReward?.() ?? 0,
		rewardStd: node.rewardStd?.() ?? 0,
		blendedValue: node.blendedValue?.() ?? 0,
		counterfactualReward: node.counterfactualReward?.() ?? 0,
		counterfactualMass: node.counterfactualMass?.() ?? 0,
		causalExpectation: node.causalExpectation?.() ?? 0,
		causalExpectationDefined: node.causalExpectationDefined?.() ?? false,
		pruned: node.pruned?.() ?? false,
		selected: node.selected?.() ?? false,
		children,
	};
};

/*
readDecisionTrace projects a telemetry decision's trace, or null when the round
did not run a search. A round without a trace is a real state — the council was
silent, or no transition model was available — and the surface says so rather
than rendering an empty tree.
*/
export const readDecisionTrace = (
	decision: Decision,
): DecisionTraceModel | null => {
	const trace = decision.trace?.();

	if (!trace) {
		return null;
	}

	const branches: TraceBranch[] = [];
	const branchCount = trace.branchesLength?.() ?? 0;

	for (let index = 0; index < branchCount; index += 1) {
		const branch = trace.branches(index);

		if (!branch) {
			continue;
		}

		branches.push({
			action: branch.action() ?? "",
			visits: Number(branch.visits()),
			meanReward: branch.meanReward(),
			blendedValue: branch.blendedValue(),
			counterfactualMass: branch.counterfactualMass(),
			counterfactualMean: branch.counterfactualMean(),
			effectiveVisits: branch.effectiveVisits(),
			causalExpectation: branch.causalExpectation(),
			causalExpectationDefined: branch.causalExpectationDefined(),
			pruned: branch.pruned(),
		});
	}

	const vetoes: string[] = [];
	const vetoCount = trace.vetoesLength?.() ?? 0;

	for (let index = 0; index < vetoCount; index += 1) {
		vetoes.push(trace.vetoes(index));
	}

	const synergies: string[] = [];
	const synergyCount = trace.synergiesLength?.() ?? 0;

	for (let index = 0; index < synergyCount; index += 1) {
		synergies.push(trace.synergies(index));
	}

	return {
		council: {
			dominantMove: trace.consensusDominantMove() ?? "",
			confidence: trace.consensusConfidence(),
			participants: Number(trace.consensusParticipants()),
			vetoes,
			synergies,
		},
		iterations: Number(trace.iterations()),
		horizon: Number(trace.horizon()),
		recommendedAction: trace.recommendedAction() ?? "",
		expectedOutcome: trace.expectedOutcome(),
		outcomeUncertainty: trace.outcomeUncertainty(),
		identificationStatus: trace.identificationStatus() ?? "",
		decisionUnavailable: trace.decisionUnavailable(),
		transitionSource: trace.transitionSource() ?? "",
		branches,
		tree: readNode(trace.tree(), 0),
	};
};
