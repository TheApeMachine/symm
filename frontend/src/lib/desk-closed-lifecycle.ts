import {
	decimal,
	positionReadout,
	settledOutcome,
} from "#/components/book-readout";
import {
	policyEpochFromRow,
	tapeMarksFromDeskRow,
} from "#/lib/desk-tape-marks";
import type { Lifecycle, LifeOutcome, LiveStopState } from "#/lib/lifecycle";
import { normalizeStoploss } from "#/lib/stoploss";

const bookFromRow = (row: Record<string, unknown>): string => {
	if (typeof row.book === "string" && row.book !== "") {
		return row.book;
	}

	return "paper";
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
	const stop = normalizeStoploss(row.stoploss) as LiveStopState | null;
	const exitTrigger =
		typeof row.exit_trigger === "string" && row.exit_trigger !== ""
			? row.exit_trigger
			: null;

	return {
		stop_state: stop,
		policy: null,
		peak_pct: stop?.peak_pnl ?? null,
		realized_pct: decimal(row.pnl_pct),
		bars_held: null,
		pnl_trace: null,
		exit_trigger: exitTrigger,
		official_exit_pct: decimal(row.pnl_pct),
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

	const entryPath =
		typeof row.entry_path === "string" ? row.entry_path : undefined;

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "holding",
		opened_at: readout.openedAt,
		closed_at: null,
		book: bookFromRow(row),
		desk: true,
		path_found: false,
		path: entryPath,
		mark: live,
		marks: live ? [live] : [],
		...stopFieldsFromRow(row),
		events: [
			{
				at: readout.openedAt,
				kind: "opened",
				detail: `desk lot opened${entryPath ? ` · ${entryPath}` : ""}`,
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
	const openDetail = `desk open · tick ${tape?.b ?? "—"}${entryPath ? ` · ${entryPath}` : ""}`;
	const exitTick = tape?.c;
	const exitTickText = exitTick === undefined ? "" : ` · tick ${exitTick}`;
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
		book: bookFromRow(row),
		desk: true,
		path_found: false,
		policy_epoch: policyEpochFromRow(row),
		path: entryPath,
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
