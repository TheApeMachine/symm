type WireRecord = Record<string, unknown>;

const WHY_LABELS: Record<string, string> = {
	"wallet.empty": "Wallet empty",
	"wallet.unfunded_quote": "Unfunded quote",
	"risk.max_open": "Max open",
	"risk.spread": "Spread",
	"risk.edge": "Edge",
	"paper.insufficient_funds": "Insufficient funds",
};

const isRecord = (raw: unknown): raw is WireRecord =>
	typeof raw === "object" && raw !== null;

const isNumberMap = (raw: unknown): raw is Record<string, number> => {
	if (!isRecord(raw)) {
		return false;
	}

	return Object.values(raw).every((value) => typeof value === "number");
};

const hasOptionalNumber = (record: WireRecord, key: string): boolean =>
	record[key] === undefined || typeof record[key] === "number";

const hasOptionalString = (record: WireRecord, key: string): boolean =>
	record[key] === undefined || typeof record[key] === "string";

const hasOptionalNumberMap = (record: WireRecord, key: string): boolean =>
	record[key] === undefined || isNumberMap(record[key]);

export type AuditEvent = {
	event: "audit";
	audit_event: string;
	seq: number;
	ts: string;
	symbol?: string;
	source?: string;
	reason?: string;
	[key: string]: unknown;
};

export type WalletPayload = {
	event?: "wallet";
	Type?: string;
	Currency?: string;
	Balance?: number;
	balance?: number;
	ReservedEUR?: number;
	FeePct?: number;
	Inventory?: Record<string, number>;
	AvgEntry?: Record<string, number>;
	Marks?: Record<string, number>;
	open?: number;
};

export type ExecutionFill = {
	channel?: "executions";
	OrderID?: string;
	order_id?: string;
	Symbol?: string;
	symbol?: string;
	Side?: string;
	side?: string;
	Qty?: number;
	qty?: number;
	Price?: number;
	price?: number;
	Fee?: number;
	fee?: number;
	reason?: string;
};

export type DecisionRow = {
	symbol: string;
	source?: string;
	score: number;
	allow: boolean;
	in_play?: boolean;
	why?: string;
};

export type DecisionTraceEvent = {
	event?: string;
	decisions: DecisionRow[];
};

export type EvaluationRow = {
	symbol: string;
	combined: number;
	support?: number;
	expected_return?: number;
	required_edge?: number;
	signals?: string[];
	allow: boolean;
	in_play?: boolean;
	why?: string;
};

const isDecisionRow = (raw: unknown): raw is DecisionRow => {
	if (!isRecord(raw)) {
		return false;
	}

	return (
		typeof raw.symbol === "string" &&
		typeof raw.score === "number" &&
		typeof raw.allow === "boolean" &&
		hasOptionalString(raw, "source") &&
		hasOptionalString(raw, "why") &&
		(raw.in_play === undefined || typeof raw.in_play === "boolean")
	);
};

export const isAuditEvent = (raw: unknown): raw is AuditEvent => {
	if (!isRecord(raw)) {
		return false;
	}

	return (
		raw.event === "audit" &&
		typeof raw.audit_event === "string" &&
		typeof raw.seq === "number" &&
		typeof raw.ts === "string" &&
		hasOptionalString(raw, "symbol") &&
		hasOptionalString(raw, "source") &&
		hasOptionalString(raw, "reason")
	);
};

export const isWalletPayload = (raw: unknown): raw is WalletPayload => {
	if (!isRecord(raw)) {
		return false;
	}

	const hasWalletMarker =
		raw.event === "wallet" ||
		raw.Type === "paper" ||
		raw.Balance !== undefined ||
		raw.balance !== undefined;

	if (!hasWalletMarker) {
		return false;
	}

	return (
		hasOptionalString(raw, "Currency") &&
		hasOptionalNumber(raw, "Balance") &&
		hasOptionalNumber(raw, "balance") &&
		hasOptionalNumber(raw, "ReservedEUR") &&
		hasOptionalNumber(raw, "FeePct") &&
		hasOptionalNumber(raw, "open") &&
		hasOptionalNumberMap(raw, "Inventory") &&
		hasOptionalNumberMap(raw, "AvgEntry") &&
		hasOptionalNumberMap(raw, "Marks")
	);
};

export const isExecutionFill = (raw: unknown): raw is ExecutionFill => {
	if (!isRecord(raw)) {
		return false;
	}

	const symbol = raw.Symbol ?? raw.symbol;
	const quantity = raw.Qty ?? raw.qty;
	const price = raw.Price ?? raw.price;

	if (typeof symbol !== "string" || typeof quantity !== "number") {
		return false;
	}

	return price === undefined || typeof price === "number";
};

export const isDecisionTraceEvent = (
	raw: unknown,
): raw is DecisionTraceEvent => {
	if (!isRecord(raw) || !Array.isArray(raw.decisions)) {
		return false;
	}

	return raw.decisions.every((decision) => isDecisionRow(decision));
};

export const whyLabel = (why: string | undefined): string => {
	if (!why) {
		return "";
	}

	return WHY_LABELS[why] ?? why.replaceAll(".", " ").replaceAll("_", " ");
};
