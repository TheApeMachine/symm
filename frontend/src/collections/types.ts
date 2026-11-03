export type Balance = {
	cash: number;
	unrealized: number;
	equity: number;
};

/*
Stoploss state as Go's StoplossState iota (broker/stoploss.go). Wire JSON
ships the numeric enum unless a string label is present.
*/
export type StoplossStateCode = 0 | 1 | 2 | 3 | 4 | 5;

export type DynamicStoplossState =
	| "Healthy"
	| "Protected"
	| "Critical"
	| "Dangerous"
	| "Triggered";

/** UI-facing stop telemetry derived from broker.Stoploss JSON. */
export type DynamicStoploss = {
	state: DynamicStoplossState;
	exit_reason?: string;
	/** Absolute entry drag (percentage points). */
	friction: number;
	current_pnl: number;
	peak_pnl: number;
	floor_pnl: number;
	has_floor: boolean;
	room_pct: number;
	proximity: number;
	/** Peak near-floor proximity [0,1] from broker.Stoploss.Damage. */
	damage?: number;
	/** Last book/tape upward probability [0,1]. */
	up_prob?: number;
	downside_pressure: number;
	downside_magnitude_ratio: number;
	consecutive_down: number;
	opened_at?: string;
	last_peak_time?: string;
};

/** Kraken spot OrderRequest as embedded on a desk lot. */
export type PositionOrder = {
	cl_ord_id?: string;
	ordertype?: string;
	type?: string;
	volume?: string;
	price?: string;
};

/** One execution fill on a desk lot (kraken.ExecutionData). */
export type PositionFill = {
	order_id?: string;
	cl_ord_id?: string;
	exec_id?: string;
	exec_type?: string;
	trade_id?: number;
	symbol?: string;
	side?: string;
	last_qty?: string | number;
	last_price?: string | number;
	cost?: string | number;
	order_type?: string;
	timestamp?: string;
	order_status?: string;
	cum_qty?: string | number;
	cum_cost?: string | number;
	avg_price?: string | number;
	fee_usd_equiv?: string | number;
};

/*
Position mirrors broker.Position JSON. Qty / entry price / fees come from
entry_order, exit_order, and fills — not duplicated scalars on the lot.
`book` is the desk ledger (virtual|paper|real), set by the broker on the wire
and stamped from the S3 tier prefix for closed-history listings.
*/
export type Position = {
	id: string;
	entry_path: string;
	hold_paths?: string[];
	exit_path: string;
	exit_trigger?: string;
	outcome_triggers?: string[];
	symbol: string;
	opened_at: string;
	closed_at?: string | null;
	updated_at?: string | null;
	policy_epoch?: number;
	entry_tick?: number;
	exit_tick?: number;
	entry_order?: PositionOrder | null;
	exit_order?: PositionOrder | null;
	fills?: PositionFill[];
	mark?: string | number;
	exit_vwap?: string | number;
	exit_room?: string | number;
	pnl?: string | number;
	pnl_pct?: string | number;
	/** Ledger stamp — from S3 scope or unlabeled→paper in onBook. */
	book?: "paper" | "virtual" | "real" | string;
	/** Raw broker.Stoploss object — normalize via normalizeStoploss. */
	stoploss?: Record<string, unknown> | DynamicStoploss;
};
