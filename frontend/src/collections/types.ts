export type Balance = {
	cash: number;
	unrealized: number;
	equity: number;
};

/*
Stoploss mirrors broker.Stoploss JSON (PascalCase tags on the Go struct).
State is StoplossState iota: Healthy=0, Armed=1, legacy=2, Triggered=3.
*/
export type StoplossState = 0 | 1 | 2 | 3;

export const STOPLOSS_HEALTHY = 0;
export const STOPLOSS_ARMED = 1;
export const STOPLOSS_TRIGGERED = 3;

export type Stoploss = {
	State: StoplossState | number;
	InitialPnLPct: number;
	CurrentPnLPct: number;
	PreviousPnLPct: number;
	PeakPnLPct: number;
	FloorPnLPct: number;
	ArmPnlPct: number;
	GivebackPnlPct: number;
	FloorMinPnlPct: number;
	TightenPerPct: number;
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
Position mirrors broker.Position JSON. Decimals arrive as strings.
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
	qty?: string | number;
	entry?: string | number;
	sold?: string | number;
	mark?: string | number;
	exit_vwap?: string | number;
	exit_room?: string | number;
	pnl?: string | number;
	pnl_pct?: string | number;
	closing?: boolean;
	tier_key?: "paper" | "virtual" | "real" | string;
	stoploss?: Stoploss;
};
