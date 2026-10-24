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
	outcome_triggers?: string[];
};
