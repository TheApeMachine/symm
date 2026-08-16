
import type { Status } from "#/types/status";

export type StopPhase = "discovery" | "protected";

export type StopTransition = {
	seq: number;
	at: string;
	phase: StopPhase;
	floor: number | string | null;
	mark: number | string | null;
	reason: string;
};

export type RiskMultiples = {
	Risk: number;
	Trail: number;
	Arm: number;
	Lock: number;
	MinEdge: number;
	MinTicks: number;
	ConfirmMarks: number;
};

export type RiskPlan = {
	present: boolean;
	entry_noise_band: number | string | null;
	noise_band: number | string | null;
	risk_distance: number | string | null;
	trail_distance: number | string | null;
	arm_buffer: number | string | null;
	lock_buffer: number | string | null;
	min_edge: number | string | null;
	max_loss: number | string | null;
	exit_fee_rate: number | string | null;
	entry_fee_rate: number | string | null;
	tick_size: number | string | null;
	confirm_marks: number;
	multiples: RiskMultiples;
};

/*
Stoploss mirrors the regulator carried by each open Holding. Optional fields
keep restored positions from older recordings readable while current live
positions expose the complete discovery/protection geometry.
*/
export type Stoploss = {
	status: Status;
	symbol: string;
	phase?: StopPhase;
	entry: number | string | null;
	qty?: number | string | null;
	entry_fee?: number | string | null;
	peak: number | string | null;
	mark: number | string | null;
	hard_floor?: number | string | null;
	profit_line?: number | string | null;
	break_even_line?: number | string | null;
	profit_failsafe?: number | string | null;
	arm_line?: number | string | null;
	profit_floor?: number | string | null;
	trail_floor?: number | string | null;
	floor: number | string | null;
	arm_at?: number | string | null;
	lock_floor?: number | string | null;
	locked?: boolean;
	profit_armed?: boolean;
	trigger_reason?: string;
	trigger_mark?: number | string | null;
	surge_armed?: boolean;
	last_move?: number | string | null;
	surge_move?: number | string | null;
	momentum_floor?: number | string | null;
	basis_confirmed?: boolean;
	plan?: RiskPlan;
	transitions?: StopTransition[];
};
