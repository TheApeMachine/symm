/*
Decision mirrors the backend types.Decision payload published on each
thesis tick so the frontend can render structural evidence, execution cost, and cause without
re-deriving gate verdicts from unrelated action frames.
*/
export type Action = "enter" | "exit" | "reduce" | "hold" | "nothing";

export type DecisionMCTSTreeNode = {
	action: number;
	actionName: string;
	depth: number;
	visits: number;
	effectiveVisits: number;
	observedReward?: number;
	counterfactualReward?: number;
	counterfactualMass?: number;
	counterfactualPrecision?: number;
	totalReward: number;
	meanReward: number;
	exploitation: number;
	exploration: number;
	causalExpectation: number;
	selectionScore: number;
	scmReady: boolean;
	scmReason?: string;
	selected: boolean;
	principal: boolean;
	state?: Record<string, number>;
	children?: DecisionMCTSTreeNode[];
};

/*
DecisionTrace mirrors Go types.DecisionTrace: the council's deliberation and
the causal search it fed. Every field here is written by strategy/trace.go on
each decision round — nothing is declared that the backend does not populate.
*/
export type DecisionTrace = {
	// Deliberation: the War Room consensus the search was handed.
	consensusDominantMove?: string;
	consensusConfidence?: number;
	consensusParticipants?: number;
	consensusProbabilities?: Record<string, number>;
	vetoes?: string[];
	synergies?: string[];
	// MCTS: what the search was configured with and what it concluded.
	identificationStatus?: string;
	decisionUnavailable?: boolean;
	expectedOutcome?: number;
	outcomeUncertainty?: number;
	horizon?: number;
	explorationConstant?: number;
	uncertaintyWeight?: number;
	transitionSource?: string;
	iterations?: number;
	recommendedAction?: string;
	maxDepth?: number;
	totalNodes?: number;
	branches?: Array<{
		action: string;
		visits: number;
		meanReward: number;
		blendedValue: number;
		rewardStd: number;
		counterfactualMass: number;
		effectiveVisits: number;
		causalExpectation: number;
		causalExpectationDefined: boolean;
		pruned: boolean;
	}>;
	tree?: DecisionMCTSTreeNode;
};

export type EntryCost = {
	entryPrice?: string;
	bestAsk?: string;
	bestBid?: string;
	midpoint?: string;
	grossNotional?: string;
	entryFee?: string;
	exitFeeAtBreakEven?: string;
	roundTripFees?: string;
	spread?: string;
	impact?: string;
	breakEven?: string;
};

/*
Decision mirrors Go types.Decision field for field. The gate sequence in
strategy/planner.go writes predictiveStatus and reason on every path, so a
rejected round explains itself as fully as an admitted one.
*/
export interface Decision {
	id: string;
	action: Action;
	symbol: string;
	at: Date;
	direction: number;
	alternatives: Record<string, number>;
	allocationClass: string;
	opportunity: boolean;
	opportunityType?: string;
	opportunityPhase?: string;
	predictiveReady: boolean;
	predictiveStatus: string;
	taskSkill: number;
	taskSkillReady: boolean;
	proposedNotional: string;
	proposedQuantity: string;
	referencePrice: string;
	forecastSource: string;
	forecastModel: string;
	forecastHorizon: number;
	calibrationCount: number;
	confidence: number;
	availableCapital: string;
	openPositions: number;
	cause: string;
	reason: string;
	reservationId?: string;
	sellableQty?: string;
	entryAt?: Date;
	exitAt?: Date;
	entryPrice?: string;
	entryFee?: string;
	exitPrice?: string;
	exitFee?: string;
	pnl?: string;
	returnPct?: number;
	mark?: string;
	entryCost?: EntryCost;
	trace?: DecisionTrace;
}

export type StrategyDecision = Decision;

/*
Finding mirrors the backend PostMortem evidence record attributed to one system
layer so proposed adjustments stay separate from live model state.
*/
export type Finding = {
	symbol: string;
	component: string;
	condition: string;
	evidence: string[];
	estimatedEffect: number;
	uncertainty: number;
	proposedAdjustment?: string;
	requiredValidation: string;
	currentModel?: string;
	candidateModel?: string;
};

export type LifecycleState = string;

export type ThesisForecast = {
	source: string;
	symbol: string;
	at: string;
	observedInterval?: number;
	sourceEpoch: number;
	horizonEvents: number;
	expiresEpoch: number;
	target: string;
	modelVersion: string;
	ready: boolean;
	calibrated: boolean;
	frictionReady: boolean;
	calibrationSamples: number;
	incrementalMSE: number;
	incrementalSkillLowerBound: number;
	referencePrice: string;
	buyCapacity: string;
	sellCapacity: string;
	expectedAdverseSelection: number;
	uncertainty: number;
	confidence: number;
};

export type ThesisHypothesis = {
	source: string;
	symbol: string;
	at: string;
	samples: number;
	ready: boolean;
	claim: string;
	treatment: string;
	controls: string[];
	outcome: string;
	association: number;
	intervention: number;
	doExpectation: number;
	uplift: number;
	counterfactual: number;
	confidence: number;
	strength: number;
};

export type ThesisCategory = {
	symbol?: string;
	type: string;
	confidence: number;
	surprisal: number;
	strength: number;
	maturity?: number;
	supporting?: string[];
	opposing?: string[];
	missing?: string[];
};

export type GraphNodeKind =
	| "measurement"
	| "category"
	| "manifold"
	| "resonance"
	| "causal"
	| "cognition"
	| "prediction"
	| "hypothesis"
	| "concept";

export type GraphNode = {
	key: string;
	kind?: GraphNodeKind;
	category?: string;
	measurement: Record<string, unknown>;
};

export type GraphEdge = {
	from: string;
	to: string;
	type: string;
	at: string;
	observedFrom: string;
};

export type Graph = {
	symbol: string;
	at: string;
	nodes: GraphNode[];
	edges: GraphEdge[];
};

/*
LIFECYCLE_STAGES preserves the backend transition order so the journal surface
can render one symbol's progress without inventing a fixed temporal window.
*/
export const LIFECYCLE_STAGES = [
	"observing",
	"shaped",
	"entry_selected",
	"entry_submitted",
	"partially_entered",
	"entered",
	"managing",
	"exit_selected",
	"exit_submitted",
	"partially_exited",
	"closed",
	"post_exit_observation",
	"postmortem_ready",
	"evaluated",
] as const;

export const LIFECYCLE_TERMINAL = new Set([
	"evaluated",
	"expired",
	"rejected",
	"invalid",
]);

const managingStart = LIFECYCLE_STAGES.indexOf("managing");
const managingEnd = LIFECYCLE_STAGES.indexOf("closed");

/*
LIFECYCLE_MANAGING spans active position-management stages between entry completion
and post-exit review so journal badges can use warning tone without index math.
*/
export const LIFECYCLE_MANAGING: Set<string> = new Set(
	managingStart >= 0 && managingEnd >= managingStart
		? LIFECYCLE_STAGES.slice(managingStart, managingEnd + 1)
		: [],
);

export const lifecycleStageIndex = (state: LifecycleState): number => {
	const index = LIFECYCLE_STAGES.indexOf(
		state as (typeof LIFECYCLE_STAGES)[number],
	);

	return index >= 0 ? index : -1;
};
