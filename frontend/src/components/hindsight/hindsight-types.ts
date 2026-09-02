/*
Hindsight read-model types: the capture tape and its persisted historical
states, exactly as the hub's /hindsight/* endpoints project the store records.
These mirror hindsight.CaptureIdentity / Run / StateEntry — not a parallel
domain model — so the UI reads the same identities the backend persisted.
*/

export type HindsightRun = {
	id: string;
	startedAt: string;
	codeCommit: string;
	buildId: string;
	configDigest: string;
	schemaVersions?: Record<string, string>;
	integrity: "COMPLETE" | "GAPPED" | "CORRUPT" | "UNKNOWN";
};

/*
HindsightRef is the full causal coordinate of one selected envelope: the
capture sequence and the deterministic ordinal within that capture. Sequence
alone is a chart rendering coordinate, never an artifact identity.
*/
export type HindsightRef = {
	sequence: number;
	ordinal: number;
};

export const sameHindsightRef = (left: HindsightRef, right: HindsightRef): boolean =>
	left.sequence === right.sequence && left.ordinal === right.ordinal;

export const compareHindsightRef = (left: HindsightRef, right: HindsightRef): number =>
	left.sequence !== right.sequence
		? left.sequence - right.sequence
		: left.ordinal - right.ordinal;

/*
orderHindsightRefs deduplicates and sorts causal coordinates by full identity:
sequence first, then ordinal. Two ordinals within one capture (100:0, 100:1) are
distinct causal points and are never collapsed into one "100".
*/
export const orderHindsightRefs = (refs: HindsightRef[]): HindsightRef[] => {
	const seen = new Map<string, HindsightRef>();

	for (const ref of refs) {
		const key = `${ref.sequence}:${ref.ordinal}`;

		if (!seen.has(key)) seen.set(key, ref);
	}

	return [...seen.values()].sort(compareHindsightRef);
};

export type HindsightCaptureIdentity = {
	run: string;
	sequence: number;
	stream: string;
	streamEpoch: number;
	streamSequence: number;
};

export type HindsightCapture = {
	identity: HindsightCaptureIdentity;
	kind: string;
	endpoint: string;
	receivedAt: string;
};

export type HindsightState = {
	envelope: {
		origin: HindsightCaptureIdentity;
		ordinal: number;
	};
	payload: string;
};

export type HindsightGap = {
	runId: string;
	encoding: string;
	sequence: number;
	detail: string;
};

/*
ExecutionFact is the venue's authoritative fill record for one execution. Every
quantity is the string the venue reported, kept verbatim rather than parsed into
a float on the wire, so no precision is lost before inspection sees it.
*/
export type HindsightExecutionFact = {
	orderId?: string;
	clientOrderId?: string;
	execId?: string;
	side?: string;
	orderStatus?: string;
	lastQty?: string;
	lastPrice?: string;
	cumQty?: string;
	cumCost?: string;
	avgPrice?: string;
	feeUsdEquiv?: string;
	fillAt?: string;
};

export type HindsightLifecycleEvent = {
	decisionId: string;
	symbol: string;
	kind: string;
	action: string;
	at: string;
	/* Present only on transitions that carried a fill. */
	execution?: HindsightExecutionFact | null;
};

export type HindsightEnvelope = {
	run: string;
	sequence: number;
	/*
		The full CaptureIdentity of the frame at this sequence, answered by the
		read itself so the inspector never has to reconstruct it from a listing
		it happens to be holding.
	*/
	capture: HindsightCapture;
	payload: string;
	manifests: Array<{
		envelope: {
			origin: HindsightCaptureIdentity;
			ordinal: number;
		};
		workload: string;
		domainKind: string;
		symbol: string;
	}>;
	witnesses: Array<{
		envelope: {
			origin: HindsightCaptureIdentity;
			ordinal: number;
		};
		boundary: string;
		artifact: { kind: string; identity: string };
		component?: string;
		componentStateVersion?: number;
		/*
			Omitted entirely when the artifact named no parent — an absent field,
			not an empty one. The witness record marks these optional, so the read
			model does too rather than assuming a shape the wire never promised.
		*/
		immediateParents?: Array<{
			origin: HindsightCaptureIdentity;
			ordinal: number;
		}>;
		semanticParents?: string[];
	}>;
};

/*
The Episode read-model: the horizontal timeline projection the hub assembles
from one Run's raw capture tape. These mirror hindsight.Timeline / Episode /
SymbolSummary exactly — the UI reads the discovery result, it does not re-derive
one, and it never relabels observed market geometry as anything a trade would
have captured.
*/

export type EpisodeKind =
	| "upward_excursion"
	| "downward_excursion"
	| "reversal"
	| "volatility_expansion"
	| "volatility_contraction"
	| "spread_expansion"
	| "liquidity_collapse"
	| "arrival_cluster";

export type ReferenceRole =
	| "anchor"
	| "peak"
	| "trough"
	| "reversal"
	| "exit_anchor"
	| "shock_onset";

export type MarketCoordinate = "midpoint" | "trade" | "last" | "bid" | "ask";

export type TimelineAxis = "capture" | "time";

export type HindsightReferencePoint = {
	role: ReferenceRole;
	capture: HindsightCaptureIdentity;
	ordinal: number;
	venueAt: string;
	receivedAt: string;
	value: number;
	hasValue: boolean;
};

export type HindsightEpisode = {
	id: string;
	symbol: string;
	kind: EpisodeKind;
	coordinate: MarketCoordinate;
	fromSequence: number;
	toSequence: number;
	fromAt: string;
	toAt: string;
	observations: number;
	observedExcursion: number;
	hasObservedExcursion: boolean;
	confirmed: boolean;
	ratio: number;
	hasRatio: boolean;
	traversed: number;
	hasTraversed: boolean;
	threshold: number;
	hasThreshold: boolean;
	references: HindsightReferencePoint[];
};

export type HindsightDiscoveryPolicy = {
	coordinate: MarketCoordinate;
	floorExcursion: number;
	excursionSigmas: number;
	excursionHorizon: number;
	retraceFraction: number;
	regimeWindow: number;
	regimeBaseline: number;
	volatilityRatio: number;
	spreadRatio: number;
	depthRatio: number;
	arrivalRatio: number;
	minRegimeSpan: number;
	minObservations: number;
	maxEpisodesPerSet: number;
};

export type HindsightDiscovery = {
	symbol: string;
	coordinate: MarketCoordinate;
	policy: HindsightDiscoveryPolicy;
	observations: number;
	defined: number;
	undefined: number;
	sigma: number;
	hasSigma: boolean;
	qualifyingMove: number;
	episodes: HindsightEpisode[];
	insufficientData: boolean;
};

export type HindsightSymbolSummary = {
	symbol: string;
	observations: number;
	defined: number;
	tickers: number;
	trades: number;
	firstSequence: number;
	lastSequence: number;
	firstAt: string;
	lastAt: string;
	episodes: number;
	insufficientData: boolean;
	topExcursion: number;
	topKind?: EpisodeKind;
	priceEpisodes: number;
	regimeEpisodes: number;
};

export type HindsightStreamSpan = {
	stream: string;
	epoch: number;
	fromSequence: number;
	toSequence: number;
	fromAt: string;
	toAt: string;
	frames: number;
	reconnect: boolean;
};

export type HindsightTimelineBucket = {
	index: number;
	fromSequence: number;
	toSequence: number;
	fromAt: string;
	toAt: string;
	observedFromSequence: number;
	observedToSequence: number;
	observedFromAt: string;
	observedToAt: string;
	observations: number;
	tickers: number;
	trades: number;
	tradeQty: number;
	defined: boolean;
	open: number;
	high: number;
	low: number;
	close: number;
	spreadFraction: number;
	hasSpreadFraction: boolean;
	touchDepth: number;
	hasTouchDepth: boolean;
	captureRate: number;
	hasCaptureRate: boolean;
};

export type HindsightTimelineSpan = {
	fromSequence: number;
	toSequence: number;
	fromAt: string;
	toAt: string;
};

export type HindsightTimeline = {
	run: string;
	symbol: string;
	coordinate: MarketCoordinate;
	policy: HindsightDiscoveryPolicy;
	axis: TimelineAxis;
	span: HindsightTimelineSpan;
	runSpan: HindsightTimelineSpan;
	buckets: HindsightTimelineBucket[];
	discovery: HindsightDiscovery;
	streams: HindsightStreamSpan[];
	symbols: HindsightSymbolSummary[];
	totalObservations: number;
	totalSymbols: number;
	indexedAt: string;
};

export type HindsightTimelineQuery = {
	run: string;
	symbol?: string;
	coordinate?: MarketCoordinate;
	axis?: TimelineAxis;
	buckets?: number;
	from?: number;
	to?: number;
	/*
		The instrument index is the same answer for every window of a run, so
		only the overview asks for it; a pan or a zoom asks for the window.
	*/
	symbols?: boolean;
};

/*
The declared semantic identity of a production metric, as METRIC_MAP.md records
it. `forbidden` is the statement that matters most on an inspection surface: it
says what may never be inferred from the number, which is a boundary this UI is
required to respect rather than a caveat it may summarise away.
*/
export type MetricSemantics = {
	identity: string;
	source: string;
	metric: string;
	class?: string;
	role?: string;
	purpose?: string;
	destinations?: string;
	forbidden?: string;
	status?: string;
};

export type HindsightMetricMap = {
	baselineCommit: string;
	metrics: Record<string, MetricSemantics>;
};

/*
The resident as-of read model.

A value here is the latest one causally available at the inspected coordinate,
which is usually not the one the inspected envelope carried. `carried` says the
value came from an earlier envelope, `ageNs` says how much earlier, and
`origin` names the exact capture it came from — so a resident value can never
be mistaken for a fresh one.

`unresolved` names the families the backward walk never found. It is not the
same claim as "the system held nothing": `exhausted` says whether the walk ran
out of budget before it ran out of history.
*/
export type ResidentMetric = {
	key: string;
	label?: string;
	raw: number;
	normalized: number;
	hasNormalized: boolean;
	standardized: number;
	hasStandardized: boolean;
	unit?: string;
	timescale?: string;
};

export type ResidentOrigin = {
	origin: HindsightCaptureIdentity;
	ordinal: number;
};

export type ResidentMeasurement = {
	source: string;
	identity?: string;
	origin: ResidentOrigin;
	atNs: number;
	ageNs: number;
	hasAge: boolean;
	carried: boolean;
	maturity: number;
	snr: number;
	snrDefined: boolean;
	metrics: ResidentMetric[];
};

export type ResidentCategory = {
	type: string;
	origin: ResidentOrigin;
	ageNs: number;
	hasAge: boolean;
	carried: boolean;
	confidence: number;
	strength: number;
	maturity: number;
	uncertainty: number;
	supporting?: string[];
	opposing?: string[];
};

export type ResidentPerspective = {
	symbol: string;
	peer?: string;
	/*
		Retired metric-bucket advisor family byte carried by historical wire data.
		This is decode-only evidence, not the current falsifiable Perspective.
	*/
	kind: number;
	origin: ResidentOrigin;
	ageNs: number;
	hasAge: boolean;
	carried: boolean;
	readings: ResidentReading[];
};

export type ResidentReading = {
	metric: string;
	value: number;
	defined: boolean;
	/* Presence flags: an undefined observation instant is absent, not zero. */
	observedAt?: number;
	hasAt: boolean;
	from?: number;
	hasFrom: boolean;
	maturity: number;
	snr: number;
	snrDefined: boolean;
};

export type HindsightResident = {
	run: string;
	symbol: string;
	sequence: number;
	ordinal: number;
	at: string;
	signals: ResidentMeasurement[];
	categories: ResidentCategory[];
	perspectives: ResidentPerspective[];
	examined: number;
	reachedBack: number;
	exhausted: boolean;
	unresolved?: string[];
};
