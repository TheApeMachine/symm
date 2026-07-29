import type {
	Finding,
	Graph,
	LifecycleState,
	StrategyDecision,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";

export type {
	Finding,
	Graph,
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
	balance: number | string;
	available?: number | string;
	reserved?: number | string;
};

export type TradeBalance = {
	e?: number | string;
	n?: number | string;
};

/*
 Holding mirrors the public JSON fields on types.Holding exactly.
 */
export type Holding = {
	status?: string;
	symbol: string;
	asset?: string;
	qty: number | string;
	sellable_qty?: number | string;
	entry_at?: string;
	exit_at?: string;
	entry_price: number | string;
	entry_fee: number | string;
	exit_price?: number | string;
	exit_fee: number | string;
	mark: number | string;
	pnl: number | string;
	return_pct: number | string;
	is_opportunity: boolean;
	reservation_id?: string;
	stoploss?: Stoploss;
};

/*
 Stoploss mirrors the public JSON fields on types.Stoploss exactly.
 */
export type Stoploss = {
	status: string;
	symbol: string;
	entry: number | string;
	peak: number | string;
	mark: number | string;
	floor: number | string;
};

export type Fill = {
	exec_id: string;
	side: string;
	qty: number | string;
	price: number | string;
	fee: number | string;
};

export type Position = {
	status: string;
	entry_order: Record<string, unknown>;
	exit_order: Record<string, unknown>;
	order_id: string;
	fills: Fill[];
	buffered: Record<string, unknown>[];
	holding: Holding;
};

export type CategoryGraphNode = {
	symbol: string;
	type: string;
	strength: number;
	freshness: number;
	at: string;
};

export type CategoryGraphEdge = {
	symbol: string;
	from: string;
	to: string;
	type: string;
	weight: number | null;
	evidence: string[];
	at: string;
};

export type CategoryGraph = {
	nodes: CategoryGraphNode[];
	edges: CategoryGraphEdge[];
	priors: Record<string, string>;
};

export type Thesis = {
	tick: number;
	at: string;
	forecasts?: ThesisForecast[];
	decisions?: StrategyDecision[];
	findings?: Finding[];
	hypotheses?: ThesisHypothesis[];
	categories?: Record<string, ThesisCategory[]>;
	positions?: Record<string, boolean>;
	holdings?: Record<string, Holding>;
	lifecycle?: Record<string, LifecycleState>;
	graphs?: Record<string, unknown> & {
		categories?: CategoryGraph;
	};
	measurements?: Record<string, unknown>;
	manifold?: Record<string, unknown>;
	cognition?: Record<string, unknown>;
	resonance?: Record<string, unknown>;
	causal?: Record<string, unknown>;
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
	last_qty?: string;
	last_price?: number;
	cost?: number;
	cum_qty?: string;
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
	oscillatorCount?: number;
	sharedOscillatorCount?: number;
	rhoOccupied?: number;
	psiOccupied?: number;
	rhoMax?: number;
	psiMax?: number;
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
	incrementalSkillLowerBound?: number;
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

export type MeasurementCategory = Omit<ThesisCategory, "maturity"> & {
	maturity?: number;
};

export type Measurement = {
	source: string;
	symbol: string;
	subject?: string;
	stream?: string;
	peer?: string;
	at: string;
	observedFrom?: string;
	horizon?: number | string;
	maturity?: number;
	uncertainty?: {
		lower?: number;
		upper?: number;
		confidence?: number;
		method?: string;
	} | null;
	metric?: string;
	side?: string;
	unit?: string;
	raw?: number;
	normalized?: number | null;
	validity: {
		state: string;
		readiness: string;
		reason?: string;
	};
	scale: {
		kind: string;
		from: string;
		through: string;
	};
	metrics?: Record<
		string,
		{
			raw: number;
			normalized?: number | null;
			unit?: string;
		}
	>;
	categories?: MeasurementCategory[];
};

export type MeasurementEpoch = {
	at: string;
	readings: Measurement[];
	publishedAt?: string;
};
