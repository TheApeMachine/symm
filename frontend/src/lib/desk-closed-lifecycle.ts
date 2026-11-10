import type { Position } from "#/collections/types";
import {
	decimal,
	holdNs,
	isClosed,
	pnlFrac,
	pnlPct,
} from "#/components/book-readout";
import {
	policyEpochFromRow,
	tapeMarksFromDeskRow,
} from "#/lib/desk-tape-marks";
import type { Lifecycle, LifeOutcome } from "#/lib/lifecycle";

const tierFrom = (pos: Position): string =>
	typeof pos.tier_key === "string" && pos.tier_key !== ""
		? pos.tier_key
		: "paper";

const stopFields = (
	pos: Position,
): Pick<
	Lifecycle,
	| "stop_state"
	| "policy"
	| "peak_pct"
	| "realized_pct"
	| "bars_held"
	| "pnl_trace"
	| "exit_trigger"
	| "official_exit_pct"
> => {
	const stop = pos.stoploss ?? null;
	const exitTrigger =
		typeof pos.exit_trigger === "string" && pos.exit_trigger !== ""
			? pos.exit_trigger
			: null;

	return {
		stop_state: stop,
		policy: null,
		peak_pct: stop?.PeakPnLPct ?? null,
		realized_pct: pnlPct(pos),
		bars_held: null,
		pnl_trace: null,
		exit_trigger: exitTrigger,
		official_exit_pct: pnlPct(pos),
	};
};

export const lifecycleFromDeskOpen = (pos: Position): Lifecycle | null => {
	if (!pos.id || !pos.opened_at || isClosed(pos)) return null;

	const mark = decimal(pos.mark);
	const qty = decimal(pos.qty) ?? 0;
	const entry = decimal(pos.entry) ?? 0;
	const frac = pnlFrac(pos) ?? 0;
	const live =
		mark === null
			? null
			: {
					at: new Date().toISOString(),
					bid: mark,
					quantity: qty,
					entry_price: entry,
					unrealized: entry * qty * frac,
					unrealized_return: frac,
				};

	return {
		id: pos.id,
		symbol: pos.symbol,
		status: "holding",
		opened_at: pos.opened_at,
		closed_at: null,
		tier_key: tierFrom(pos),
		desk: true,
		path_found: false,
		path: pos.entry_path || undefined,
		mark: live,
		marks: live ? [live] : [],
		...stopFields(pos),
		events: [
			{
				at: pos.opened_at,
				kind: "opened",
				detail: `desk lot opened${pos.entry_path ? ` · ${pos.entry_path}` : ""}`,
			},
		],
	};
};

export const lifecycleFromDeskClosed = (pos: Position): Lifecycle | null => {
	if (!pos.id || !pos.opened_at || !isClosed(pos) || !pos.closed_at) {
		return null;
	}

	const frac = pnlFrac(pos);
	const pct = pnlPct(pos);
	const money = decimal(pos.pnl);
	const triggers =
		pos.outcome_triggers?.filter((t) => typeof t === "string") ?? [];
	const held = holdNs(pos) ?? 0;

	const outcome =
		frac !== null && pct !== null
			? ({
					pnl: frac,
					pnl_pct: pct,
					money,
					hold_ns: held,
					expected_hold_ns: null,
					triggers: triggers.length > 0 ? triggers : (["desk settle"] as string[]),
				} satisfies LifeOutcome)
			: null;

	const tape = tapeMarksFromDeskRow(pos);
	const exitTick = tape?.c;
	const exitTickText = exitTick === undefined ? "" : ` · tick ${exitTick}`;
	const closeDetail =
		triggers.length > 0
			? `${triggers.join(", ")}${exitTickText}`
			: pct === null
				? `closed${exitTickText}`
				: `return ${pct.toFixed(2)}%${exitTickText}`;

	return {
		id: pos.id,
		symbol: pos.symbol,
		status: "closed",
		opened_at: pos.opened_at,
		closed_at: pos.closed_at,
		tier_key: tierFrom(pos),
		desk: true,
		path_found: false,
		policy_epoch: policyEpochFromRow(pos),
		path: pos.entry_path || undefined,
		marks_a: tape?.a ?? null,
		marks_b: tape?.b ?? null,
		marks_c: tape?.c ?? null,
		...stopFields(pos),
		events: [
			{
				at: pos.opened_at,
				kind: "opened",
				detail: `desk open · tick ${tape?.b ?? "—"}${pos.entry_path ? ` · ${pos.entry_path}` : ""}`,
			},
			{
				at: pos.closed_at,
				kind: "closed",
				detail: closeDetail,
			},
		],
		outcome,
	};
};
