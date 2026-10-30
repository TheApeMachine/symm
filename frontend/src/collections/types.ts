/*
Position shapes crossing the wire.

A desk position arrives as one Value frame per lot whose "position" layer
holds the broker's Position JSON: the desk emits the frame with the symbol as
scope and the decoded position object as the layer payload, so the fields here
mirror the JSON exactly.
*/
export type DeskPosition = {
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
	Stop-loss and exit-policy telemetry the desk publishes on every mark.
	Mirrors broker/stoploss.go StopState and the learned exit policy on
	broker/position.go. Decimals cross the wire as JSON strings, so they are
	string|number here and parsed by the decimal() helper on read.
	*/
	/** Live proximity of each of the four stop checks, refreshed per mark. */
	stop_state?: {
		invalidation?: number;
		trailing?: number;
		evaporation?: number;
		expiry?: number;
		peak_pct?: number;
		current_pct?: number;
		bars_held?: number;
		max_bars?: number;
	} | null;
	/** Highest mark-to-market return (percent) the lot has reached. */
	peak_pnl_pct?: string | number;
	/** Learned trailing giveback (percent) off the peak, from Causal MCTS. */
	exit_pullback_pct?: string | number;
	/** Learned invalidation cut (percent) below entry, from Causal MCTS. */
	nursery_cut_pct?: string | number;
	/** Empirical winning horizon in reprice bars (move-expiry ceiling). */
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
};
