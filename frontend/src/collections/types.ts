import type { Category, Measurement } from "#/types/measurement";
import type {
	Finding,
	GraphFrame,
	LifecycleState,
	StrategyDecision,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";

export type {
	Category,
	Measurement,
	Finding,
	GraphFrame,
	LifecycleState,
	StrategyDecision,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
};

/*
Balance mirrors the Balance.Frame wallet row JSON.
*/
export type Balance = {
	asset: string;
	balance: number;
	available: number;
	reserved: number;
};

/*
Holding mirrors types.Holding JSON from Balance.Frame.
*/
export type Holding = {
	symbol: string;
	qty: number;
	entry_price: number;
	entry_fee: number;
	exit_price?: number;
	exit_fee: number;
	mark: number;
	pnl: number;
	return_pct: number;
	status?: string;
	asset?: string;
	stop_price?: number;
	stop_return?: number;
	peak_return?: number;
	stop_armed?: boolean;
	momentum_active?: boolean;
	momentum_health?: number;
	stagnation_active?: boolean;
	stagnation_health?: number;
	stagnation_pending?: boolean;
};

/*
Stop mirrors Stoploss.Frame JSON published beside holdings.
*/
export type Stop = {
	symbol: string;
	stop_price: number;
	peak_return: number;
	stop_return: number;
	armed?: boolean;
	peak_price?: number;
	momentum?: number;
	peak_momentum?: number;
	momentum_floor?: number;
	momentum_health?: number;
	momentum_active?: boolean;
	peak_touch_count?: number;
	stagnation_max_touches?: number;
	stagnation_health?: number;
	stagnation_pending?: boolean;
	stagnation_active?: boolean;
};

/*
Order mirrors an open-order row on the UI wire.
*/
export type Order = {
	id: string;
	pair: string;
	price: number;
	reserved_amount: number;
	reserved_asset: string;
	side: string;
	type: string;
	volume: number;
	created_at: string;
};

/*
Execution mirrors a flat desk execution row on the UI wire.
*/
export type Execution = {
	exec_id?: string;
	order_id?: string;
	symbol?: string;
	side?: string;
	order_status?: string;
	exec_type?: string;
	timestamp?: string;
	last_qty?: number;
	last_price?: number;
	cost?: number;
	cum_qty?: number;
	cum_cost?: number;
	avg_price?: number;
	fee_usd_equiv?: number;
};

/*
Instrument is one traded pair row on the UI wire.
*/
export type Instrument = Record<string, unknown> & {
	symbol: string;
};

/*
LifecycleRow is one symbol's lifecycle phase on the UI wire.
*/
export type LifecycleRow = {
	symbol: string;
	state: LifecycleState;
};

/*
TickFrame is the coalesced engine tick snapshot on the UI wire.
*/
export type TickFrame = Record<string, unknown> & {
	count?: number;
	open?: number;
	candidates?: number;
	quotes_ready?: number;
	quotes_total?: number;
};

/*
DiagnosticsFrame is a cross-section diagnostics snapshot on the UI wire.
*/
export type DiagnosticsFrame = Record<string, unknown>;

/*
DashboardFrame is a generic keyed UI frame used by legacy paint helpers.
*/
export type DashboardFrame = Record<string, unknown>;

/*
CausalReading mirrors algorithm.PearlOutput published inside each causal frame.
*/
export type CausalReading = {
	value?: number;
	category?: number;
	confidence?: number;
	confidenceBaseline?: number;
	entryBaseline?: number;
	exitBaseline?: number;
	strength?: number;
	association?: number;
	associationScore?: number;
	intervention?: number;
	interventionScore?: number;
	doExpectation?: number;
	uplift?: number;
	upliftScore?: number;
	counterfactual?: number;
	noise?: number;
	contagion?: number;
	condition?: number;
	inverted?: boolean;
	probabilities?: number[];
	distribution?: Record<string, number>;
};

/*
CausalFrame mirrors logic.CausalOutcome published on each thesis tick.
*/
export type CausalFrame = {
	source: string;
	symbol: string;
	at: string;
	samples?: number;
	ready?: boolean;
	hypothesis?: string;
	treatment?: string;
	controls?: string[];
	target?: string;
	reading?: CausalReading;
};

/*
ManifoldFrame mirrors the backend manifold wire payload.
*/
export type ManifoldFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
};

export type ResonanceLayer = {
	state: number[];
	prediction: number[];
};

/*
ResonanceFrame mirrors the backend ResonanceOutcome wire payload.
*/
export type ResonanceFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
	samples?: number;
	observables?: number[];
	latent?: number[];
	layers?: ResonanceLayer[];
	energy?: number;
	surprise?: number;
	expectedReturn?: number;
	returnReady?: boolean;
	incrementalMSE?: number;
	uncertainty?: number;
	calibrationSamples?: number;
};

/*
CognitiveReading mirrors the backend cognition payload on each Thesis tick.
*/
export type CognitiveReading = {
	symbol: string;
	scope?: string;
	sequence?: string;
	regimePrefix?: string;
	regimeCohort?: number;
	ambiguous?: boolean;
	sideline?: boolean;
	entropyBits?: number;
	entropyThreshold?: number;
	classConfidence?: number;
	contrastEvidence?: number;
	lookaheadScore?: number;
	lookaheadPaths?: number;
	winnerClass?: string;
	prewarmPaths?: number | null;
	prewarmScore?: number | null;
	updatedAt?: number;
	beamWidth?: number;
	maxHops?: number;
	nodeCount?: number;
	branches?: CognitiveBranch[];
	beams?: CognitiveBeam[];
	classes?: CognitiveClass[];
	remFrom?: string;
	remThrough?: string;
	remReplays?: number;
	at?: string;
	winner?: string;
	ready?: boolean;
	confidence?: number;
	contrast?: number;
	cohort?: number;
};

export type CognitiveBranch = {
	id: number;
	parentId: number;
	token: string;
	prefix: string;
	depth: number;
	probability: number;
	count: number;
};

export type CognitiveBeam = {
	sequence: string;
	score: number;
};

export type CognitiveClass = {
	name: string;
	probability: number;
};

/*
MeasurementEpoch groups measurements that share an observation timestamp for
readers that still join multi-metric frames by tick.
*/
export type MeasurementEpoch = {
	at: string;
	readings: Measurement[];
	publishedAt: string;
};
