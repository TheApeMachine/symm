import type { Stoploss } from "#/types/stoploss";
import type {
	Decision,
	Finding,
	Graph,
	LifecycleState,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";

export type {
	Finding,
	Graph,
	LifecycleState,
	Decision,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
	Stoploss,
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
	decisions?: Decision[];
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
CausalReading mirrors algorithm.PearlOutput.

The solver publishes it flat — every reading sits at the top level of the row
alongside the symbol, and the baselines keep their snake-cased wire names. A
nested `reading` wrapper and camel-cased baselines were what this file used to
describe, which is why every consumer of a causal row read undefined.
*/
export type CausalReading = {
	value?: number;
	category?: number;
	confidence?: number;
	confidence_baseline?: number;
	entry_baseline?: number;
	exit_baseline?: number;
	strength?: number;
	association?: number;
	associationScore?: number;
	intervention?: number;
	interventionScore?: number;
	doExpectation?: number;
	uplift?: number;
	upliftScore?: number;
	residual?: number;
	counterfactual?: number;
	noise?: number;
	contagion?: number;
	condition?: number;
	inverted?: number;
	probabilities?: number[];
	distribution?: Record<string, number>;
};

/*
CausalFrame mirrors logic.CausalOutcome published on each thesis tick. A row
that is not ready yet carries only its symbol and that flag.
*/
export type CausalFrame = CausalReading & {
	source?: string;
	symbol: string;
	at?: string;
	samples?: number;
	ready?: boolean;
	hypothesis?: string;
	treatment?: string;
	controls?: string[];
	target?: string;
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
	errorNorm?: number;
	temporal?: boolean;
};

export type ResonanceForecast = {
	forwardCurve: number[];
	forwardRetention: number[];
	supportedHorizon: number;
	expectedReturn: number;
	expectedBasisPoints: number;
	confidence: number;
	confidenceReady: boolean;
	predictiveScale?: number;
	predictiveScaleBasisPoints?: number;
	degreesOfFreedom?: number;
};

/*
ResonanceVerdict mirrors the backend types.ResonanceVerdict. The labels are
decided by the solver, never rederived here.
*/
export type ResonanceVerdict = {
	learning: "observing" | "predicting";
	tuning: "recursive least squares";
	learningHealth: number;
	tuningHealth: number;
	direction: number;
	conviction: number;
};

export type ForecastValidity = {
	state: "valid" | "provisional" | "invalid";
	readiness: string;
	reason?: string;
};

/*
ResonanceFrame mirrors the backend ResonanceOutcome wire payload.
*/
export type ResonanceFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
	samples?: number;
	skillEvidence?: number;
	observables?: number[];
	latent?: number[];
	embedding?: number[];
	layers?: ResonanceLayer[];
	energy?: number;
	surprise?: number;
	targetSymbol?: string;
	forecast?: ResonanceForecast;
	verdict?: ResonanceVerdict;
	forecastValidity?: ForecastValidity;
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
	ambiguous?: boolean;
	sideline?: boolean;
	entropyBits?: number;
	entropyThreshold?: number;
	lookaheadScore?: number;
	lookaheadPaths?: number;
	predictions?: Record<string, CognitivePrediction>;
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

/*
CognitivePrediction is one projected path the beam search kept, keyed by the
path itself in the predictions map.
*/
export type CognitivePrediction = {
	predictedPath?: string;
	probability?: number;
	hops?: number;
	score?: number;
};

export type CognitiveBranch = {
	id: number;
	parentId: number;
	token: string;
	prefix: string;
	key: string;
	depth: number;
	probability: number;
	count: number;
};

export type CognitiveBeam = {
	sequence: string;
	key: string;
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
