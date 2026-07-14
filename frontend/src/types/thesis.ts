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
	availableCapital: number;
	openPositions: number;
	slotCapacity: number;
	cause: string;
	reason: string;
};

/*
TradeObservation mirrors the backend immutable broker and position facts that
append to one thesis lifecycle in publication order.
*/
export type TradeObservation = {
	kind: string;
	action?: string;
	symbol: string;
	side?: string;
	status?: string;
	orderId?: string;
	executionId?: string;
	quantity?: string;
	price?: string;
	cost?: string;
	fee?: string;
	pnl?: string;
	returnPct?: number;
	decision: number;
	error?: string;
	at: string;
};

/*
Finding mirrors the backend PostMortem evidence record attributed to one system
layer so proposed adjustments stay separate from live model state.
*/
export type Finding = {
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

export const lifecycleStageIndex = (state: LifecycleState): number => {
	const index = LIFECYCLE_STAGES.indexOf(
		state as (typeof LIFECYCLE_STAGES)[number],
	);

	return index >= 0 ? index : -1;
};
