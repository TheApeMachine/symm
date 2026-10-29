import {
	decimal,
	positionReadout,
	settledOutcome,
} from "#/components/book-readout";
import type {
	Lifecycle,
	LifeOutcome,
	LiveStopState,
	LearnedExitPolicy,
} from "#/lib/lifecycle";
import {
	policyEpochFromRow,
	tapeMarksFromDeskRow,
} from "#/lib/desk-tape-marks";
/*
Desk lifecycle rows mirror the broker position JSON. A row without a
complete close or stable identity is not a closed round trip.
*/
/*
stopStateFromRow parses the stop-check proximity object the desk publishes on
a raw row. A non-object stop_state is null so the UI reads "no stop telemetry
yet" rather than inventing proximity.
*/
const stopStateFromRow = (
	row: Record<string, unknown>,
): LiveStopState | null => {
	const value = row.stop_state;
	if (!value || typeof value !== "object") return null;
	const record = value as Record<string, unknown>;
	const num = (key: string): number | undefined => {
		const item = record[key];
		return typeof item === "number" && Number.isFinite(item)
			? item
			: undefined;
	};
	return {
		invalidation: num("invalidation"),
		trailing: num("trailing"),
		evaporation: num("evaporation"),
		expiry: num("expiry"),
		peak_pct: num("peakPct") ?? num("peak_pct"),
		current_pct: num("currentPct") ?? num("current_pct"),
		bars_held: num("barsHeld") ?? num("bars_held"),
		max_bars: num("maxBars") ?? num("max_bars"),
	};
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

/*
stopFieldsFromRow gathers the stop and exit-policy fields off a raw desk row
for the lifecycle. Everything is optional: a lot restored without a book, or a
virtual trial opened moments ago, carries none of them and the UI shows the
meters in their waiting state rather than a fabricated reading.
*/
const stopFieldsFromRow = (
	row: Record<string, unknown>,
): Pick<
	Lifecycle,
	"stop_state" | "policy" | "peak_pct" | "realized_pct" | "bars_held" | "pnl_trace" | "exit_trigger"
> => {
	const peakPct = decimal(row.peak_pnl_pct);
	const realized = decimal(row.realized_pct);
	const barsHeld = decimal(row.bars_held);
	const trace = Array.isArray(row.pnl_trace)
		? row.pnl_trace.filter(
				(v): v is number =>
					typeof v === "number" && Number.isFinite(v),
			)
		: null;
	const exitTrigger =
		typeof row.exit_trigger === "string" && row.exit_trigger !== ""
			? row.exit_trigger
			: null;

	return {
		stop_state: stopStateFromRow(row),
		policy: policyFromRow(row),
		peak_pct: peakPct,
		realized_pct: realized,
		bars_held: barsHeld !== null && barsHeld > 0 ? barsHeld : null,
		pnl_trace: trace,
		exit_trigger: exitTrigger,
	};
};

/*
lifecycleFromDeskOpen is an open desk lot as a lifecycle row: the life-cycle
tab shows the lot holding (with its live mark) until the close frame lands.
*/
export const lifecycleFromDeskOpen = (
	row: Record<string, unknown>,
): Lifecycle | null => {
	if (!row.id) return null;

	const readout = positionReadout(row);
	if (readout.closed || readout.openedAt === null) return null;

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

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "holding",
		opened_at: readout.openedAt,
		closed_at: null,
		book: row.virtual ? "virtual" : "paper",
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
				detail: row.virtual ? "virtual trial opened" : "desk lot opened",
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

	const outcome =
		pnl !== null && pnlPct !== null
			? ({
					pnl,
					pnl_pct: pnlPct,
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
	const exitTickText =
		exitTick === undefined ? "" : ` · tick ${exitTick}`;
	const closeDetail =
		triggers.length > 0
			? `${triggers.join(", ")}${exitTickText}`
			: pnlPct === null
				? `closed${exitTickText}`
				: `return ${pnlPct.toFixed(2)}%${exitTickText}`;

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "closed",
		opened_at: readout.openedAt,
		closed_at: readout.closedAt,
		book: row.virtual ? "virtual" : "paper",
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
