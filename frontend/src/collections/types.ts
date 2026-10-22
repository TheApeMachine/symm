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
	entry_order: Record<string, unknown> | null;
	exit_order: Record<string, unknown> | null;
	entry_order_result: Record<string, unknown> | null;
	exit_order_result: Record<string, unknown> | null;
	holding: Holding;
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

export type LifecycleState = string;

/*
DeskPosition is one broker.Position as JSON on the tick's positions layer.
Field names mirror the Go struct's json tags exactly. Decimals arrive as
strings. closed_at is null while the lot is open.
*/
export type DeskPosition = {
	id: string;
	path?: string;
	symbol: string;
	opened_at?: string | null;
	closed_at?: string | null;
	entry_tick?: number | string | null;
	exit_tick?: number | string | null;
	virtual?: boolean;
	qty?: string | null;
	entry_price?: string | null;
	exit_price?: string | null;
	entry_fee?: string | null;
	exit_fee?: string | null;
	mark?: string | null;
	exit_vwap?: string | null;
	exit_room?: number | string | null;
	return?: number | null;
	hold_ns?: number | null;
	cost?: string | null;
	proceeds?: string | null;
	profit?: string | null;
	net_return?: number | null;
	outcome_triggers?: string[] | null;
	fills?: Array<Record<string, unknown>> | null;
	entry_orders?: Array<Record<string, unknown>> | null;
	exit_orders?: Array<Record<string, unknown>> | null;
	/** The priced close (broker settle): never zero when unpriced — the triggers say why. */
	outcome?: {
		cost: string | null;
		proceeds: string | null;
		fees: string | null;
		realized: string | null;
		net_return: number | null;
		hold_ns: number;
		expected_hold_ns: number | null;
		triggers: string[] | null;
	} | null;
	/**
	Tape ticks of a closed round trip: A some prints before the entry (B),
	C at the exit — the Position Life-Cycle's training-sample markers.
	*/
	marks?: { a: number | null; b: number | null; c: number | null } | null;
	/** The run epoch of the round trip: the tape the A B C markers index. */
	run?: string | null;
};

export class DecisionT {
	id = "";
	symbol = "";
	action = "";
	cause = "";
	reason = "";
	confidence = 0;

	constructor(init: Partial<DecisionT> = {}) {
		Object.assign(this, init);
	}
}

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
	taskForecast?: number;
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

/*
TradeRecord mirrors the JSON shape of wire.PositionT as returned by the hub's
GET /trades endpoint (broker.PositionStore.RecentTrades, backed by the
position_trades SQLite table) — the durable trade journal, independent of the
live positions atom.
*/
export type TradeRecord = {
	status: string;
	decision?:
		| (Record<string, unknown> & {
				id?: string;
				// The opportunity this entry was positioning for, how far the
				// planner's gate sequence got, and the council's confidence.
				opportunityType?: string;
				opportunityPhase?: string;
				predictiveReady?: boolean;
				predictiveStatus?: string;
				confidence?: number;
				calibrationCount?: number;
				forecastSource?: string;
				forecastModel?: string;
				forecastHorizon?: number;
				cause?: string;
				reason?: string;
				entryCost?: {
					bestAsk?: string;
					bestBid?: string;
					spread?: string;
					impact?: string;
					breakEven?: string;
					roundTripFees?: string;
				} | null;
				risk?: {
					riskDistance?: string;
					trailDistance?: string;
					armBuffer?: string;
					lockBuffer?: string;
					maxLoss?: string;
					minEdge?: string;
				} | null;
				trace?: {
					// What the council concluded and what the causal search did
					// with it. Written by strategy/trace.go on every round.
					consensusDominantMove?: string;
					consensusConfidence?: number;
					consensusParticipants?: number;
					vetoes?: string[] | null;
					synergies?: string[] | null;
					recommendedAction?: string;
					identificationStatus?: string;
					decisionUnavailable?: boolean;
					transitionSource?: string;
					iterations?: number;
					horizon?: number;
					expectedOutcome?: number;
					outcomeUncertainty?: number;
					maxDepth?: number;
					totalNodes?: number;
				} | null;
		  })
		| null;
	holding?: {
		symbol?: string;
		status?: string;
		entryAt?: number;
		exitAt?: number;
		entryPrice?: string;
		entryFee?: string;
		exitPrice?: string;
		exitFee?: string;
		pnl?: string;
		returnPct?: number;
		stoploss?: {
			status?: string;
			floor?: string;
			peak?: string;
			profitLine?: string;
			locked?: boolean;
			triggerReason?: string;
			triggerMark?: string;
			surgeArmed?: boolean;
		} | null;
	} | null;
};

export class PositionT {
	status = "";
	holding: HoldingT | null = null;

	constructor(init: Partial<PositionT> = {}) {
		Object.assign(this, init);
	}
}
export class SellEventT {
	trigger = "";
	qty = "";
	at: bigint = 0n;
	capacityQty = 0;
	capacityRatio = 0;
	budget: number | string = 0;
	note = "";

	constructor(init: Partial<SellEventT> = {}) {
		Object.assign(this, init);
	}
}
export class HoldingT {
	symbol = "";
	status = "";
	pnl: number | string = "0";
	entryPrice: number | string = "0";
	mark: number | string = "0";
	returnPct = 0;
	capacityDefined = false;
	capacityRatio: number | string = 0;
	capacityBounded = false;
	slippageBudget: number | string = 0;
	budgetSource = "";
	venuePnl: number | string = "0";
	shadowPnl: number | string = "0";
	shadowDefined = false;
	closedAt: bigint = 0n;
	sells: SellEventT[] = [];

	constructor(init: Partial<HoldingT> = {}) {
		Object.assign(this, init);
	}
}
