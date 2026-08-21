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
	qty: number | string | null;
	sellable_qty?: number | string | null;
	entry_at?: string;
	exit_at?: string;
	entry_price: number | string | null;
	entry_fee: number | string | null;
	exit_price: number | string | null;
	exit_fee: number | string | null;
	mark: number | string | null;
	pnl: number | string | null;
	profit_threshold: number | string | null;
	return_pct: number | string;
	is_opportunity: boolean;
	reservation_id?: string;
	stoploss: Stoploss | null;
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
	decision: Decision;
	entry_order: Record<string, unknown> | null;
	exit_order: Record<string, unknown> | null;
	entry_order_result: Record<string, unknown> | null;
	exit_order_result: Record<string, unknown> | null;
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
LaneSnapshot is one bounded stream edge on the diagnostics wire. It describes
which components push into and pull from a queue, its current depth, and the
capacity budget when bounded.
*/
export type LaneSnapshot = {
	name: string;
	kind: string;
	writers: string[];
	readers: string[];
	depth: number;
	cap?: number;
	high_water: number;
	blocking: boolean;
	symbols?: number;
	saturations: number;
	saturation_ns: number;
};

/*
DiagnosticsFrame is the stream-pipeline snapshot on the UI wire. It reports
sequencer lag and lane pressure, not signal activity.
*/
export type DiagnosticsFrame = {
	status?: "flowing" | "queued" | "stalled" | "lossy";
	summary?: string;
	ingress_sequence?: number;
	committed_sequence?: number;
	next_sequence?: number;
	lag?: number;
	pending?: number;
	dropped?: number;
	commit_dropped?: number;
	tickers?: number;
	books?: number;
	trades?: number;
	level3?: number;
	coalesced_books?: number;
	stall_ns?: number;
	ui_depth?: number;
	ui_cap?: number;
	ui_sent?: number;
	ui_dropped?: number;
	lanes?: LaneSnapshot[];
	queues?: QueueSnapshot[];
	lossy?: boolean;
	at_ns?: number;
	started_ns?: number;
	stages?: ClockSnapshot[];
	hops?: HopSnapshot[];
	errors?: ErrorSnapshot[];
	pass?: PassStatus;
};

/*
QueueSnapshot is one observable queue's live pressure read on the wire. It names
which components write into and read from the queue, its current item depth,
capacity when bounded, and the peak depth observed during the session.
*/
export type QueueSnapshot = {
	name: string;
	kind: "ingress" | "rail" | "derived" | "strategy" | "ui" | "broker";
	writers: string[];
	readers: string[];
	depth: number;
	cap?: number;
	high_water: number;
	symbols?: number;
};

export type ClockSnapshot = {
	name: string;
	kind?: string;
	count: number;
	total_ns: number;
	last_ns: number;
	max_ns?: number;
	last_at_ns?: number;
	active?: number;
	started_ns?: number;
};

export type HopSnapshot = {
	from: string;
	to: string;
	count: number;
	total_ns: number;
	last_ns: number;
	max_ns?: number;
};

export type ErrorSnapshot = {
	source: string;
	message: string;
	caller?: string;
	at_ns?: number;
};

export type PassStatus = {
	state: "idle" | "running" | "blocked";
	in_flight_ns?: number;
	last_pass_ns?: number;
	since_last_ns?: number;
};

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
CausalFrame mirrors the causal estimate published for each aligned observation.
Precision reports finite-sample support without withholding an estimate.
*/
export type CausalFrame = CausalReading & {
	source?: string;
	symbol: string;
	at?: string;
	samples?: number;
	precision?: number;
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
	symbol?: string;
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
	probeHorizon?: number;
	aggregate?: {
		Value?: number;
		Scale?: number;
		DegreesOfFreedom?: number;
		Ready?: boolean;
	};
	posterior?: Array<{
		Value?: number;
		Scale?: number;
		DegreesOfFreedom?: number;
		Ready?: boolean;
	}>;
};

export type ResonanceDynamics = {
	ready?: number;
	deltaTime?: number;
	position?: number;
	velocity?: number;
	acceleration?: number;
	memory?: number;
	memoryScale?: number;
	storedEnergy?: number;
	suppliedPower?: number;
	dissipation?: number;
	passivityResidue?: number;
	continuousVariance?: number;
	jumpAmplitude?: number;
	jumpVariance?: number;
	sampleCount?: number;
	rotorScalar?: number;
	rotorBivector?: number;
	equivarianceNorm?: number;
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
	taskRelativePrecision?: number;
	taskRelativePrecisionReady?: boolean;
	taskCalibration?: "calibrating" | "calibrated";
	taskSkill?: number;
	taskSkillReady?: boolean;
	taskSkillStatus?:
		| "calibrating"
		| "below baseline"
		| "baseline"
		| "above baseline";
	lastResolvedForecast?: number;
	lastRealizedReturn?: number;
	lastForecastError?: number;
	observables?: number[];
	latent?: number[];
	embedding?: number[];
	layers?: ResonanceLayer[];
	energy?: number;
	surprise?: number;
	targetSymbol?: string;
	forecast?: ResonanceForecast;
	dynamics?: ResonanceDynamics;
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
	candidateWinner?: string;
	stateHeld?: boolean;
	predictionsHeld?: boolean;
	switchConfidence?: number;
	switchThreshold?: number;
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
	id?: string;
	source: string;
	symbol: string;
	tick?: number;
	peer?: string;
	at: string;
	observedFrom?: string;
	peerAt?: string;
	peerObservedFrom?: string;
	horizon?: number | string;
	maturity?: number;
	uncertainty?: {
		lower?: number;
		upper?: number;
		confidence?: number;
		method?: string;
	} | null;
	metrics?: Record<
		string,
		{
			raw: number;
			normalized?: number | null;
			unit?: string;
		}
	>;
	metadata?: Record<string, number>;
	categories?: MeasurementCategory[];
};

export type MeasurementEpoch = {
	at: string;
	readings: Measurement[];
	publishedAt?: string;
};
