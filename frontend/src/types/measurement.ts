/*
Category mirrors types.Category from the Go backend.
Each measurement carries an array of categories — one per regime class
the signal classifies into — with a posterior probability (confidence),
the raw signal strength, and a surprisal value relative to the signal's
own threshold.
*/
export type Category = {
	type: string;
	confidence: number;
	surprisal: number;
	strength: number;
};

/*
Measurement mirrors types.Measurement from the Go backend.
Measurements arrive inside the backend-owned UI frame under the
`measurements` key. The WebSocket provider ingests that named batch
without inferring message type from array shape.

source     — signal origin (e.g. "fluid", "hawkes", "correlation")
symbol     — trading pair (e.g. "BTC/EUR")
at         — ISO 8601 timestamp of the measurement
status     — e.g. "measured"
elapsed    — seconds since the signal last fired
entryBaseline — dynamic entry threshold from the classifier
exitBaseline  — dynamic exit threshold from the classifier
categories — posterior over regime classes
metrics    — arbitrary signal-specific numerics (map[string]float64)
*/
export type Measurement = {
	source: string;
	symbol: string;
	at: string;
	status: string;
	elapsed: number;
	entryBaseline: number;
	exitBaseline: number;
	categories: Category[];
	metrics: Record<string, number>;
};

/*
Known source types emitted by the backend signals.
Not exhaustive — new signals may appear; the frontend routes by
string matching, not by enum membership.
*/
export const KNOWN_SOURCES = [
	"causal",
	"correlation",
	"cvd",
	"depthflow",
	"exhaustion",
	"fluid",
	"hawkes",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
	"toxicity",
] as const;

export type SourceType = (typeof KNOWN_SOURCES)[number] | string;
