/** Wire protocol events pushed over ws://host/ws */

export type SymmEventName =
	| "hello"
	| "engine_pulse"
	| "field_snapshot"
	| "field_row"
	| "field_grid"
	| "candle_bar";

export type SymmEvent = {
	event: SymmEventName;
	ts: string;
	[key: string]: unknown;
};

export type FluidSymbolRow = {
	symbol: string;
	change_pct: number;
	vol: number;
	div: number;
	vort: number;
	turb: number;
	visc: number;
	re: number;
};

export type FluidGridPayload = {
	size: number;
	heights: number[][];
	min: number;
	max: number;
	filled_cells: number;
	outliers: {
		clipped_count: number;
		clipped_at: number;
		raw_max: number;
		raw_max_symbol?: string;
		display_max: number;
	};
};

export type FieldSnapshotEvent = SymmEvent & {
	event: "field_snapshot";
	symbol_count: number;
	symbols: FluidSymbolRow[];
	grid?: FluidGridPayload;
};

export type FieldRowEvent = SymmEvent & {
	event: "field_row";
	symbol: string;
	row: FluidSymbolRow;
};

export type FieldGridEvent = SymmEvent & {
	event: "field_grid";
	grid: FluidGridPayload;
};

export type EnginePulseEvent = SymmEvent & {
	event: "engine_pulse";
	seq: number;
	phase: string;
	measurements: number;
	candidates?: number;
	open: number;
	ticker_ready?: number;
	symbols_total?: number;
	fluid_sampled?: number;
	avg_prediction?: number;
	avg_error?: number;
	forecast_symbols?: number;
};

export type DecisionRow = {
	symbol: string;
	regime: string;
	reason: string;
	score: number;
	allow: boolean;
	why: string;
	confidence: number;
};

export type EvaluationRow = {
	symbol: string;
	combined: number;
	allow: boolean;
	why: string;
	signals: { source: string; confidence: number }[];
};

export type DecisionTraceEvent = SymmEvent & {
	event: "decision_trace";
	decisions: DecisionRow[];
	evaluations?: EvaluationRow[];
};

export const whyLabel = (code: string): string => code.replaceAll("_", " ");

export type WalletPayload = {
	Type?: number;
	Currency?: string;
	Balance?: number;
	ReservedEUR?: number;
	FeePct?: number;
	Inventory?: Record<string, number>;
};

export type ExecutionFill = {
	OrderID: string;
	Symbol: string;
	Side: string;
	Qty: number;
	Price: number;
};

export type PredictionFeedback = {
	Source: string;
	Symbol: string;
	PredictedReturn: number;
	ActualReturn: number;
	Error: number;
	Confidence?: number;
};

export const isPredictionFeedback = (
	raw: unknown,
): raw is PredictionFeedback => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		typeof row.Source === "string" &&
		typeof row.Symbol === "string" &&
		typeof row.PredictedReturn === "number" &&
		typeof row.ActualReturn === "number" &&
		typeof row.Error === "number"
	);
};

export const eventTimeSec = (event: SymmEvent): number => {
	const parsed = Date.parse(event.ts);

	return Number.isFinite(parsed)
		? Math.floor(parsed / 1000)
		: Math.floor(Date.now() / 1000);
};

export const isEnginePulseEvent = (raw: unknown): raw is EnginePulseEvent => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return row.event === "engine_pulse" && typeof row.seq === "number";
};

export const isFieldRowEvent = (raw: unknown): raw is FieldRowEvent => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		row.event === "field_row" &&
		typeof row.symbol === "string" &&
		typeof row.row === "object" &&
		row.row !== null
	);
};

export const isFieldSnapshotEvent = (
	raw: unknown,
): raw is FieldSnapshotEvent => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return row.event === "field_snapshot" && Array.isArray(row.symbols);
};

export const isFieldGridEvent = (raw: unknown): raw is FieldGridEvent => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		row.event === "field_grid" &&
		typeof row.grid === "object" &&
		row.grid !== null
	);
};

export const isWalletPayload = (raw: unknown): raw is WalletPayload => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		typeof row.Balance === "number" &&
		typeof row.Inventory === "object" &&
		row.Inventory !== null
	);
};

export const isExecutionFill = (raw: unknown): raw is ExecutionFill => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		typeof row.OrderID === "string" &&
		typeof row.Symbol === "string" &&
		typeof row.Side === "string" &&
		typeof row.Qty === "number" &&
		typeof row.Price === "number"
	);
};

export const isHelloEvent = (
	raw: unknown,
): raw is SymmEvent & { event: "hello" } => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	return (raw as Record<string, unknown>).event === "hello";
};
