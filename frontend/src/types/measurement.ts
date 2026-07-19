/*
Category is a logic-layer classification for one symbol. Interpretation happens
after signal conditioning: the analyzer classifies from cognition and attaches
the measurement evidence that supports, opposes, or is missing for the category
from the symbol's composed evidence graph. Supporting/opposing/missing carry the
measurement keys behind the label so the terminal shows why a category is lit.
*/
export type Category = {
	symbol?: string;
	type: string;
	confidence: number;
	surprisal: number;
	strength: number;
	maturity?: number;
	supporting?: string[];
	opposing?: string[];
	missing?: string[];
};

export type MeasurementUncertainty = {
	lower?: number;
	upper?: number;
	confidence?: number;
	method?: string;
};

export type MeasurementValidity = {
	state: string;
	readiness: string;
	reason?: string;
};

export type ScaleReference = {
	kind: string;
	from: string;
	through: string;
};

/*
Measurement mirrors types.Measurement from the Go backend. Numerical identity,
units, time scale, and validity remain intact on the frontend; the optional
compatibility fields exist only for signals that have not migrated yet.
*/
export type Measurement = {
	source: string;
	metric?: string;
	subject?: string;
	stream?: string;
	symbol: string;
	side?: string;
	at: string;
	observedFrom?: string;
	horizon?: number;
	unit?: string;
	raw: number;
	normalized: number | null;
	maturity?: number;
	uncertainty: MeasurementUncertainty | null;
	validity: MeasurementValidity;
	scale: ScaleReference;

	status?: string;
	elapsed?: number;
	entryBaseline?: number;
	exitBaseline?: number;
	categories?: Category[];
	metrics?: Record<string, number>;
};

/*
Known source types emitted by the backend signals. The frontend still accepts
new source names because source identity is data, not a closed UI enum.
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
