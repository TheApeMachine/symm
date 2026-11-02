export type Balance = {
	cash: number;
	unrealized: number;
	equity: number;
};

export type DynamicStoplossState =
	| "Healthy"
	| "Protected"
	| "Stalled"
	| "Dangerous"
	| "Triggered";

/** Live dynamic stop composed on an open lot (broker DynamicStoploss JSON). */
export type DynamicStoploss = {
	state: DynamicStoplossState;
	exit_reason?: string;
	/** Percentage points. */
	friction: number;
	current_pnl: number;
	peak_pnl: number;
	floor_pnl: number;
	has_floor: boolean;
	room_pct: number;
	proximity: number;
	downside_pressure: number;
	downside_magnitude_ratio: number;
	consecutive_down: number;
	opened_at?: string;
	last_peak_time?: string;
};

/*
Position shapes crossing the wire.

A desk position arrives as one Value frame per lot whose "position" layer
holds the broker's Position JSON: the desk emits the frame with the symbol as
scope and the decoded position object as the layer payload, so the fields here
mirror the JSON exactly.
*/
export type Position = {
	id: string;
	entry_path: string;
	hold_path?: string;
	exit_path: string;
	symbol: string;
	opened_at: string;
	closed_at?: string | null;
	policy_epoch?: number;
	mark_a_tick?: number;
	entry_tick?: number;
	exit_tick?: number;
	qty?: string | number;
	entry_price?: string | number;
	exit_price?: string | number;
	entry_fee?: string | number;
	exit_fee?: string | number;
	mark?: string | number;
	exit_vwap?: string | number;
	exit_room?: string | number;
	pnl?: string | number;
	pnl_pct?: string | number;
	open_pnl?: string | number;
	hold_ns?: number;
	virtual?: boolean;
	/** True when virtual enter matched a stored prefix, not a first-time explore step. */
	path_found?: boolean;
	/** Human-readable reason the exit fired, e.g. "invalidation stop: …". */
	exit_trigger?: string;
	outcome_triggers?: string[];

	/** Highest mark-to-market return (percent) the lot has reached. */
	peak_pnl_pct?: string | number;
	/** Legacy learned trailing giveback (percent); unused by the fixed stop. */
	exit_pullback_pct?: string | number;
	/** Legacy learned invalidation cut (percent); unused by the fixed stop. */
	nursery_cut_pct?: string | number;
	/** Legacy empirical winning horizon in reprice bars. */
	max_lifetime_bars?: number;
	/** Legacy empirical duration ceiling in nanoseconds. */
	max_lifetime_ns?: number;
	/** Worst return ever realized on this symbol, feeds the learned horizon. */
	worst_pct?: string | number;
	/** Repress events the lot has held (tape timebase, not wall clock). */
	bars_held?: number;
	/** Fee-net realized return captured at exit (fraction, not percent). */
	realized_pct?: number;
	/** Net percent path, one mark per reprice — the episode the policy learns from. */
	pnl_trace?: number[];
	official_exit_at?: string | null;
	official_exit_pct?: number;
	stoploss?: DynamicStoploss;
};
