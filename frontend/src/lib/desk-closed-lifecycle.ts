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
				detail: `desk position opened${pos.entry_path ? ` · ${pos.entry_path}` : ""}`,
			},
		],
	};
};

/** Fill-realized $ / % when S3 closed rows omit pnl (flat Qty cannot re-mark). */
const fillRealized = (
	pos: Position,
): { money: number; pct: number; frac: number } | null => {
	let buyQty = 0;
	let buyNotional = 0;
	let sellNotional = 0;
	let fees = 0;

	for (const fill of pos.fills ?? []) {
		const qty = decimal(fill.last_qty);
		const px = decimal(fill.last_price);
		if (qty === null || qty <= 0 || px === null) continue;

		const notional = px * qty;
		const fee = decimal(fill.fee_usd_equiv);
		if (fee !== null && fee > 0) fees += fee;

		if (fill.side === "buy") {
			buyQty += qty;
			buyNotional += notional;
		} else if (fill.side === "sell") {
			sellNotional += notional;
		}
	}

	if (buyQty <= 0 || buyNotional <= 0) return null;

	const money = sellNotional - buyNotional - fees;
	const pct = (money / buyNotional) * 100;
	return { money, pct, frac: pct / 100 };
};

export const lifecycleFromDeskClosed = (pos: Position): Lifecycle | null => {
	if (!pos.id || !pos.opened_at || !isClosed(pos) || !pos.closed_at) {
		return null;
	}

	const fromFills = fillRealized(pos);
	// Prefer fill-realized over stored pnl_pct — a stale mark crest on the
	// row (peak left after a flat reopen) must not show as a +35% win.
	const pct = fromFills?.pct ?? pnlPct(pos) ?? null;
	const money = fromFills?.money ?? decimal(pos.pnl) ?? null;
	const frac =
		fromFills?.frac ??
		pnlFrac(pos) ??
		(pct !== null ? pct / 100 : null);
	const triggers =
		pos.outcome_triggers?.filter((t) => typeof t === "string") ?? [];
	const held = holdNs(pos) ?? 0;

	// Prefer pct/frac; fall back to money-only / fill-realized so blank S3 rows show $.
	const outcome: LifeOutcome | null =
		frac !== null || money !== null
			? {
					pnl: frac ?? 0,
					pnl_pct: pct ?? (frac !== null ? frac * 100 : 0),
					money,
					hold_ns: held,
					expected_hold_ns: null,
					triggers: triggers.length > 0 ? triggers : (["desk settle"] as string[]),
				}
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
