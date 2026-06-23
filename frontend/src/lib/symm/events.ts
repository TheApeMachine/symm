const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null && !Array.isArray(value);

const finiteNumber = (value: unknown): number | undefined => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return undefined;
	}

	return value;
};

const stringField = (
	record: Record<string, unknown>,
	key: string,
): string | undefined => {
	const value = record[key];

	if (typeof value !== "string" || value.trim() === "") {
		return undefined;
	}

	return value;
};

const numberMap = (value: unknown): Record<string, number> | undefined => {
	if (!isRecord(value)) {
		return undefined;
	}

	const output: Record<string, number> = {};

	for (const [key, raw] of Object.entries(value)) {
		const parsed = finiteNumber(raw);

		if (parsed === undefined) {
			continue;
		}

		output[key] = parsed;
	}

	return output;
};

export type WalletPayload = {
	Type?: string;
	Currency?: string;
	Balance?: number;
	Inventory?: Record<string, number>;
	AvgEntry?: Record<string, number>;
	Marks?: Record<string, number>;
	ExpectedExit?: Record<string, number>;
	Unrealized?: Record<string, number>;
	ReservedEUR?: number;
	FeePct?: number;
	Realized?: number;
};

export type ExecutionFill = {
	OrderID: string;
	Symbol: string;
	Side: string;
	Qty: number;
	Price: number;
};

export type AuditEvent = {
	event?: string;
	audit_event: string;
	seq: number;
	ts: string;
	symbol?: string;
	source?: string;
	reason?: string;
	why?: string;
	playbook?: string;
	perspectives?: string[];
	conviction?: number;
	edge?: number;
	confidence?: number;
	predicted_return?: number;
	actual_return?: number;
	net_return?: number;
	forward_return?: number;
	error?: string;
	urgency?: string;
	success?: boolean;
	held_ms?: number;
	slot_eur?: number;
	spread_bps?: number;
	fill_price?: number;
};

export type DecisionTraceEvent = {
	type?: string;
	story_ticks?: number;
	playbook_evaluations?: number;
	decisions?: Array<{
		symbol: string;
		source?: string;
		score: number;
		allow: boolean;
		in_play: boolean;
		why?: string;
		signals?: Array<{
			source: string;
			confidence: number;
		}>;
	}>;
};

export type EvaluationRow = {
	symbol: string;
	combined: number;
	support?: number;
	expected_return?: number;
	required_edge?: number;
	signals?: Array<{
		source: string;
		confidence: number;
	}>;
	allow: boolean;
	why?: string;
};

export const isWalletPayload = (raw: unknown): raw is WalletPayload => {
	if (!isRecord(raw)) {
		return false;
	}

	const frameType = stringField(raw, "type");

	if (frameType === "balances" || frameType === "wallet") {
		return true;
	}

	return (
		raw.Type === "paper" ||
		raw.Inventory !== undefined ||
		raw.Balance !== undefined
	);
};

export const isExecutionFill = (raw: unknown): raw is ExecutionFill => {
	if (!isRecord(raw)) {
		return false;
	}

	const symbol = stringField(raw, "Symbol");
	const orderId = stringField(raw, "OrderID");
	const side = stringField(raw, "Side");
	const qty = finiteNumber(raw.Qty);
	const price = finiteNumber(raw.Price);

	return (
		symbol !== undefined &&
		orderId !== undefined &&
		side !== undefined &&
		qty !== undefined &&
		price !== undefined
	);
};

export const isAuditEvent = (raw: unknown): raw is AuditEvent => {
	if (!isRecord(raw)) {
		return false;
	}

	const auditEvent = stringField(raw, "audit_event");

	if (auditEvent === undefined) {
		return false;
	}

	const seq = finiteNumber(raw.seq);
	const ts = stringField(raw, "ts");

	return seq !== undefined && ts !== undefined;
};

export const isDecisionTraceEvent = (
	raw: unknown,
): raw is DecisionTraceEvent => {
	if (!isRecord(raw)) {
		return false;
	}

	if (raw.type === "decision_walk" || raw.type === "decision_trace") {
		return Array.isArray(raw.decisions);
	}

	return Array.isArray(raw.decisions);
};

export const walletPayloadFromFrame = (
	raw: Record<string, unknown>,
): WalletPayload => ({
	Type: stringField(raw, "Type") ?? stringField(raw, "type"),
	Currency: stringField(raw, "Currency"),
	Balance: finiteNumber(raw.Balance),
	Inventory: numberMap(raw.Inventory),
	AvgEntry: numberMap(raw.AvgEntry),
	Marks: numberMap(raw.Marks),
	ExpectedExit: numberMap(raw.ExpectedExit),
	Unrealized: numberMap(raw.Unrealized),
	ReservedEUR: finiteNumber(raw.ReservedEUR),
	FeePct: finiteNumber(raw.FeePct),
	Realized: finiteNumber(raw.Realized),
});

export const whyLabel = (value: string | undefined) => {
	if (value === undefined || value.trim() === "") {
		return "—";
	}

	return value.replaceAll("_", " ");
};
