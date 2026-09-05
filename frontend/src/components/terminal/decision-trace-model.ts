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

export type AdvisorClass = {
	state: string;
	probability: number;
};

export type AdvisorMoveMass = {
	move: string;
	mass: number;
};

export type AdvisorOpinion = {
	advisor: string;
	state: string;
	probability: number;
	credibility: number;
	weight: number;
	/* The whole reading, not only the class that led it. */
	classes: AdvisorClass[];
	maturity: number;
	/* The move mass this advisor alone placed, before normalization. */
	contribution: AdvisorMoveMass[];
	/* Its classes no projection rule accepted, whose weight was discarded. */
	unmapped: string[];
	/* Classes it declares but could not measure; they took no part in the reading. */
	unscored: string[];
	clock: string;
	leaseFrom: number;
	leaseUntil: number;
	clockNow: number;
};

/*
Why an advisor contributed nothing. "incomplete" means its declared evidence
has never all arrived for this symbol, which is a wiring fault that persists
silently; "expired" means it published and the clock has passed its lease,
which is the ordinary rhythm of a slow instrument.
*/
export type AdvisorSilence = {
	advisor: string;
	reason: string;
	missing: string[];
	declared: number;
	leaseUntil: number;
	clockNow: number;
};

export type CouncilTrace = {
	dominantMove: string;
	confidence: number;
	participants: number;
	vetoes: string[];
	synergies: string[];
	advisors: AdvisorOpinion[];
	silent: AdvisorSilence[];
	unmappedClasses: string[];
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

	const advisors: AdvisorOpinion[] = [];
	const advisorCount = trace.advisorsLength?.() ?? 0;

	for (let index = 0; index < advisorCount; index += 1) {
		const opinion = trace.advisors(index);

		if (!opinion) {
			continue;
		}

		const classes: AdvisorClass[] = [];

		for (let entry = 0; entry < (opinion.classesLength?.() ?? 0); entry += 1) {
			const reading = opinion.classes(entry);

			if (!reading) continue;

			classes.push({
				state: reading.state() ?? "",
				probability: reading.probability(),
			});
		}

		const contribution: AdvisorMoveMass[] = [];

		for (
			let entry = 0;
			entry < (opinion.contributionLength?.() ?? 0);
			entry += 1
		) {
			const mass = opinion.contribution(entry);

			if (!mass) continue;

			contribution.push({ move: mass.move() ?? "", mass: mass.mass() });
		}

		const unmapped: string[] = [];

		for (let entry = 0; entry < (opinion.unmappedLength?.() ?? 0); entry += 1) {
			unmapped.push(opinion.unmapped(entry));
		}

		const unscored: string[] = [];

		for (let entry = 0; entry < (opinion.unscoredLength?.() ?? 0); entry += 1) {
			unscored.push(opinion.unscored(entry));
		}

		advisors.push({
			advisor: opinion.advisor() ?? "",
			state: opinion.state() ?? "",
			probability: opinion.probability(),
			credibility: opinion.credibility(),
			weight: opinion.weight(),
			classes,
			maturity: opinion.maturity(),
			contribution,
			unmapped,
			unscored,
			clock: opinion.clock() ?? "",
			leaseFrom: Number(opinion.leaseFrom()),
			leaseUntil: Number(opinion.leaseUntil()),
			clockNow: Number(opinion.clockNow()),
		});
	}

	const silent: AdvisorSilence[] = [];

	for (
		let index = 0;
		index < (trace.advisorSilencesLength?.() ?? 0);
		index += 1
	) {
		const entry = trace.advisorSilences(index);

		if (!entry) continue;

		const missing: string[] = [];

		for (let key = 0; key < (entry.missingLength?.() ?? 0); key += 1) {
			missing.push(entry.missing(key));
		}

		silent.push({
			advisor: entry.advisor() ?? "",
			reason: entry.reason() ?? "",
			missing,
			declared: entry.declared(),
			leaseUntil: Number(entry.leaseUntil()),
			clockNow: Number(entry.clockNow()),
		});
	}

	const unmappedClasses: string[] = [];

	for (
		let index = 0;
		index < (trace.consensusUnmappedClassesLength?.() ?? 0);
		index += 1
	) {
		unmappedClasses.push(trace.consensusUnmappedClasses(index));
	}

	return {
		council: {
			dominantMove: trace.consensusDominantMove() ?? "",
			confidence: trace.consensusConfidence(),
			participants: Number(trace.consensusParticipants()),
			vetoes,
			synergies,
			advisors,
			silent,
			unmappedClasses,
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
