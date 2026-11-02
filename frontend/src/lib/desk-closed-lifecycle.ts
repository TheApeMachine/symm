import {
	decimal,
	positionReadout,
	settledOutcome,
} from "#/components/book-readout";
import {
	policyEpochFromRow,
	tapeMarksFromDeskRow,
} from "#/lib/desk-tape-marks";
import type {
	DynamicStoploss,
	DynamicStoplossState,
} from "#/collections/types";
import type {
	LearnedExitPolicy,
	Lifecycle,
	LifeOutcome,
	LiveStopState,
} from "#/lib/lifecycle";

/*
Desk lifecycle rows mirror the broker position JSON. A row without a
complete close or stable identity is not a closed round trip.
*/

const nest = (
	row: Record<string, unknown>,
	key: string,
): Record<string, unknown> => {
	const value = row[key];
	return value && typeof value === "object"
		? (value as Record<string, unknown>)
		: {};
};

const STOP_STATES: DynamicStoplossState[] = [
	"Healthy",
	"Protected",
	"Stalled",
	"Dangerous",
	"Triggered",
];

const stopStateFromRow = (
	row: Record<string, unknown>,
): LiveStopState | null => {
	const sl = nest(row, "stoploss");
	const state = sl.state;
	if (typeof state !== "string" || !STOP_STATES.includes(state as DynamicStoplossState)) {
		return null;
	}

	const num = (key: string): number | undefined => {
		const item = sl[key];
		return typeof item === "number" && Number.isFinite(item) ? item : undefined;
	};

	const friction = num("friction");
	const current = num("current_pnl");
	const peak = num("peak_pnl");
	const floor = num("floor_pnl");
	if (
		friction === undefined ||
		current === undefined ||
		peak === undefined ||
		floor === undefined
	) {
		return null;
	}

	const stop: DynamicStoploss = {
		state: state as DynamicStoplossState,
		friction,
		current_pnl: current,
		peak_pnl: peak,
		floor_pnl: floor,
		has_floor: sl.has_floor === true,
		room_pct: num("room_pct") ?? 0,
		proximity: num("proximity") ?? 0,
		downside_pressure: num("downside_pressure") ?? 0,
		downside_magnitude_ratio: num("downside_magnitude_ratio") ?? 0,
		consecutive_down: num("consecutive_down") ?? 0,
	};

	if (typeof sl.exit_reason === "string" && sl.exit_reason !== "") {
		stop.exit_reason = sl.exit_reason;
	}
	if (typeof sl.opened_at === "string" && sl.opened_at !== "") {
		stop.opened_at = sl.opened_at;
	}
	if (typeof sl.last_peak_time === "string" && sl.last_peak_time !== "") {
		stop.last_peak_time = sl.last_peak_time;
	}

	return stop;
};

/*
policyFromRow lifts the learned exit policy off a raw row. Each learned
boundary is a JSON decimal string on the wire; absent or zero fields stay
null so the UI can say "not learned yet" instead of a fabricated 0%.
*/
const policyFromRow = (
	row: Record<string, unknown>,
): LearnedExitPolicy | null => {
	const trailing = decimal(row.exit_pullback_pct);
	const cut = decimal(row.nursery_cut_pct);
	const maxBars = decimal(row.max_lifetime_bars);
	const worst = decimal(row.worst_pct);
	const hasAny =
		(trailing ?? 0) > 0 ||
		(cut ?? 0) > 0 ||
		(maxBars ?? 0) > 0 ||
		worst !== null;
	if (!hasAny) return null;
	return {
		trailing: trailing !== null && trailing > 0 ? trailing : null,
		cut: cut !== null && cut > 0 ? cut : null,
		maxBars: maxBars !== null && maxBars > 0 ? maxBars : null,
		worst,
	};
};

const finiteNums = (raw: unknown): number[] | null => {
	if (!Array.isArray(raw)) return null;
	const next = raw.filter(
		(value): value is number => typeof value === "number" && Number.isFinite(value),
	);
	return next.length > 0 ? next : null;
};

const stopFieldsFromRow = (
	row: Record<string, unknown>,
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
	const peakPct = decimal(row.peak_pnl_pct);
	const realized = decimal(row.realized_pct);
	const barsHeld = decimal(row.bars_held);
	const trace = finiteNums(row.pnl_trace);
	const exitTrigger =
		typeof row.exit_trigger === "string" && row.exit_trigger !== ""
			? row.exit_trigger
			: null;
	const official =
		typeof row.official_exit_pct === "number" &&
		Number.isFinite(row.official_exit_pct)
			? row.official_exit_pct
			: null;

	return {
		stop_state: stopStateFromRow(row),
		policy: policyFromRow(row),
		peak_pct: peakPct,
		realized_pct: realized,
		bars_held: barsHeld !== null && barsHeld > 0 ? barsHeld : null,
		pnl_trace: trace,
		exit_trigger: exitTrigger,
		official_exit_pct: official,
	};
};

export const lifecycleFromDeskOpen = (
	row: Record<string, unknown>,
): Lifecycle | null => {
	if (!row.id) return null;

	const readout = positionReadout(row);
	if (readout.openedAt === null) return null;
	if (readout.closed) return null;

	const mark = readout.mark;
	const live =
		mark === null
			? null
			: {
					at: new Date().toISOString(),
					bid: mark,
					quantity: readout.qty ?? 0,
					entry_price: readout.entryPrice ?? 0,
					unrealized:
						readout.returnPct !== null &&
						readout.entryPrice !== null &&
						readout.qty !== null
							? readout.entryPrice * readout.qty * readout.returnPct
							: 0,
					unrealized_return: readout.returnPct ?? 0,
				};

	const openDetail = row.virtual ? "virtual trial opened" : "desk lot opened";

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "holding",
		opened_at: readout.openedAt,
		closed_at: null,
		book:
			typeof row.book === "string" && row.book !== ""
				? (row.book as string)
				: row.virtual
					? "virtual"
					: "paper",
		desk: true,
		path_found: row.path_found === true,
		path: row.path as string | undefined,
		mark: live,
		marks: live ? [live] : [],
		...stopFieldsFromRow(row),
		events: [
			{
				at: readout.openedAt,
				kind: "opened",
				detail: openDetail,
			},
		],
	};
};

export const lifecycleFromDeskClosed = (
	row: Record<string, unknown>,
): Lifecycle | null => {
	if (!row.id) return null;

	const readout = positionReadout(row);
	if (
		!readout.closed ||
		readout.openedAt === null ||
		readout.closedAt === null
	) {
		return null;
	}

	const settle = settledOutcome(row);
	const pnl = readout.returnPct ?? settle?.pnl ?? null;
	const pnlPct = readout.returnPctDisplay ?? settle?.pnl_pct ?? null;
	const money = decimal(row.pnl);

	const outcome =
		pnl !== null && pnlPct !== null
			? ({
					pnl,
					pnl_pct: pnlPct,
					money,
					hold_ns:
						settle?.hold_ns ??
						(readout.closedAt !== null && readout.openedAt !== null
							? (Date.parse(readout.closedAt) - Date.parse(readout.openedAt)) *
								1e6
							: 0),
					expected_hold_ns: null,
					triggers: settle?.triggers ?? (["desk settle"] as string[]),
				} satisfies LifeOutcome)
			: null;

	const tape = tapeMarksFromDeskRow(row);
	const entryPath =
		typeof row.entry_path === "string" ? row.entry_path : undefined;
	const triggers = settle?.triggers ?? [];
	const openDetail = row.virtual
		? `virtual open · tick ${tape?.b ?? "—"}${entryPath ? ` · ${entryPath}` : ""}`
		: `desk open · tick ${tape?.b ?? "—"}`;
	const exitTick = tape?.c;
	const exitTickText = exitTick === undefined ? "" : ` · tick ${exitTick}`;
	const officialPct =
		typeof row.official_exit_pct === "number" &&
		Number.isFinite(row.official_exit_pct)
			? row.official_exit_pct
			: null;
	const exitAtText =
		officialPct !== null ? ` · exit ${officialPct.toFixed(2)}%` : "";
	const closeDetail =
		triggers.length > 0
			? `${triggers.join(", ")}${exitTickText}${exitAtText}`
			: pnlPct === null
				? `closed${exitTickText}${exitAtText}`
				: `return ${pnlPct.toFixed(2)}%${exitTickText}${exitAtText}`;

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "closed",
		opened_at: readout.openedAt,
		closed_at: readout.closedAt,
		book:
			typeof row.book === "string" && row.book !== ""
				? (row.book as string)
				: row.virtual
					? "virtual"
					: "paper",
		desk: true,
		path_found: row.path_found === true,
		policy_epoch: policyEpochFromRow(row),
		path: entryPath ?? (row.path as string | undefined),
		marks_a: tape?.a ?? null,
		marks_b: tape?.b ?? null,
		marks_c: tape?.c ?? null,
		...stopFieldsFromRow(row),
		events: [
			{
				at: readout.openedAt,
				kind: "opened",
				detail: openDetail,
			},
			{
				at: readout.closedAt,
				kind: "closed",
				detail: closeDetail,
			},
		],
		outcome,
	};
};
