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
This is the ONLY shape the backend sends over the WebSocket — an array
of these, serialized with sonic.Marshal. No envelope, no wrapper.

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

/*
isMeasurement is a type guard for incoming WebSocket messages.
*/
export const isMeasurement = (value: unknown): value is Measurement =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof (value as Record<string, unknown>).source === "string" &&
	typeof (value as Record<string, unknown>).symbol === "string" &&
	Array.isArray((value as Record<string, unknown>).categories);

/*
parseMeasurements validates and filters an incoming WebSocket payload.
The backend sends JSON: Measurement[] (a flat array).
*/
export const parseMeasurements = (data: unknown): Measurement[] => {
	if (!Array.isArray(data)) {
		return [];
	}

	return data.filter(isMeasurement);
};

/*
bestCategory returns the category with the highest confidence from
a measurement's categories array. Mirrors logic/boundary.go bestCategory.
*/
export const bestCategory = (categories: Category[]): Category | null => {
	if (categories.length === 0) {
		return null;
	}

	let best = categories[0];

	for (let index = 1; index < categories.length; index++) {
		if (categories[index].confidence > best.confidence) {
			best = categories[index];
		}
	}

	return best;
};
