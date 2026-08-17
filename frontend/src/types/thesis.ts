
import type { Status } from "#/types/status";
import type { RiskPlan, Stoploss } from "#/types/stoploss";

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

export type DecisionTrace = {
	hypothesis?: string;
	graphSupports: number;
	graphContradicts: number;
	graphConditions: number;
	thesisBalance: number;
	thesisConfidence: number;
	mcts: {
		iterations: number;
		branches: Array<{
			action: string;
			visits: number;
			meanReward: number;
		}>;
		recommendedAction?: string;
		tree?: DecisionMCTSTreeNode;
		maxDepth?: number;
		totalNodes?: number;
	};
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

export interface Decision {
	id: string;
	action: Action;
	symbol: string;
	at: Date;
	utility: number;
	graphScore: number;
	thesisScore: number;
	thesisConfidence: number;
	thesisSupport: number;
	thesisContradiction: number;
	thesisConditions: number;
	direction: number;
	allocation_haircut: number;
	allocation_haircut_reason: string;
	alternatives: Record<string, number>;
	allocationClass: string;
	opportunity: boolean;
	opportunityType?: string;
	reserveEligible: boolean;
	reserveReason?: string;
	predictiveReady: boolean;
	predictiveStatus: string;
	taskSkill: number;
	taskSkillReady: boolean;
	proposedNotional: string;
	proposedQuantity: string;
	referencePrice: string;
	validThroughEpoch: number;
	arbitrationRound?: number;
	forecastSource: string;
	forecastModel: string;
	forecastEpoch: number;
	forecastHorizon: number;
	calibrationCount: number;
	expectedReturn: string;
	expectedFees: string;
	expectedSpread: string;
	expectedImpact: string;
	adverseSelection: string;
	uncertainty: number;
	confidence: number;
	causalPrecision: number;
	opportunityMargin: number;
	cognitiveLead: number;
	basinConfidence: number;
	availableCapital: string;
	openPositions: number;
	slotCapacity: number;
	cause: string;
	reason: string;
	displaces?: string;
	displacedQuantity?: string;
	displacedPrice?: string;
	reservationId?: string;
	positionStatus?: Status;
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
	stoploss?: Stoploss;
	risk?: RiskPlan;
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
	expectedReturn: number;
	referencePrice: string;
	buyCapacity: string;
	sellCapacity: string;
	expectedFees: number;
	expectedSpread: number;
	expectedImpact: number;
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
