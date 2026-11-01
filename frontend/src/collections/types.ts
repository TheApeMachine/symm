export type Balance = {
	cash: number;
	unrealized: number;
	equity: number;
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

	/*
	Stop-loss telemetry the desk publishes on every mark. Mirrors
	broker/stoploss.go StopState. Decimals cross the wire as JSON strings, so
	they are string|number here and parsed by the decimal() helper on read.
	*/
	/** Live proximity of invalidation / trailing / damage, refreshed per mark. */
	stop_state?: {
		invalidation?: number;
		trailing?: number;
		damage?: number;
		peak_pct?: number;
		current_pct?: number;
		entry_pct?: number;
		giveback_pct?: number;
		allowed_giveback_pct?: number;
		remaining_giveback_pct?: number;
		floor_pct?: number;
		floorPct?: number;
	} | null;
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
	/** Full structural-shadow series (shared prefix with pnl_trace + post-official continuation). */
	shadow_pnl_trace?: number[];
	/** True while the virtual structural ghost still wants marks. */
	shadow_active?: boolean;
	shadow_exit_trigger?: string;
	official_exit_at?: string | null;
	shadow_exit_at?: string | null;
	official_exit_pct?: number;
	shadow_exit_pct?: number;
	/** Active official stop rail in PnL % (trail only while green). */
	stop_floor_pct?: number;
	/** Live structural shadow floor in PnL %. */
	shadow_floor_pct?: number;
	/** Official floor path, one sample per reprice. */
	stop_floor_trace?: number[];
	/** Structural floor path, one sample per shadow mark. */
	shadow_floor_trace?: number[];
	/** 0..1+ toward the active official stop. */
	stop_proximity?: number;
	/** PnL points remaining above the active official floor. */
	stop_room_pct?: number;
	shadow_proximity?: number;
	shadow_room_pct?: number;
};
