/*
StrategyDecision mirrors the backend types.Decision payload published on each
thesis tick so the frontend can render utility, alternatives, and cause without
re-deriving gate verdicts from unrelated action frames.
*/
export type StrategyDecision = {
	action: string;
	symbol: string;
	at: string;
	utility: number;
	alternatives: Record<string, number>;
	allocationClass: string;
	proposedNotional: number;
	proposedQuantity: number;
	referencePrice: number;
	validThroughEpoch: number;
	forecastSource: string;
	forecastModel: string;
	forecastEpoch: number;
	calibrationCount: number;
	expectedReturn: number;
	expectedFees: number;
	expectedSpread: number;
	expectedImpact: number;
	adverseSelection: number;
	uncertainty: number;
	confidence: number;
	opportunityMargin: number;
	cognitiveLead: number;
	basinConfidence: number;
	availableCapital: number;
	openPositions: number;
	slotCapacity: number;
	cause: string;
	reason: string;
};

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
	referencePrice: number;
	buyCapacity: number;
	sellCapacity: number;
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
	maturity: number;
	supporting?: string[];
	opposing?: string[];
	missing?: string[];
};

export type GraphNodeKind = "measurement" | "category" | "concept";

export type GraphNodeWire = {
	key: string;
	kind?: GraphNodeKind;
	category?: string;
	measurement: Record<string, unknown>;
};

export type GraphEdgeWire = {
	from: string;
	to: string;
	type: string;
	at: string;
	observedFrom: string;
};

export type GraphFrame = {
	symbol: string;
	at: string;
	nodes: GraphNodeWire[];
	edges: GraphEdgeWire[];
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
