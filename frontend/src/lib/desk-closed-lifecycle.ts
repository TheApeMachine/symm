import {
	decimal,
	positionReadout,
	settledOutcome,
} from "#/components/book-readout";
import type { Lifecycle, LifeOutcome } from "#/lib/lifecycle";
import {
	policyEpochFromRow,
	tapeMarksFromDeskRow,
} from "#/lib/desk-tape-marks";
/*
Desk lifecycle rows mirror the broker position JSON. A row without a
complete close or stable identity is not a closed round trip.
*/
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
					unrealized: readout.netPnl ?? 0,
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
		path: row.path as string | undefined,
		mark: live,
		marks: live ? [live] : [],
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
	const settledReturn = settle ? decimal(settle.net_return) : null;
	const netReturn =
		readout.returnPct ??
		(settledReturn === null ? null : settledReturn / 100);

	const outcome =
		settle && netReturn !== null
			? {
				cost: settle.cost,
				proceeds: settle.proceeds,
				fees: 0,
				realized: settle.realized,
				net_return: netReturn,
				hold_ns:
					settle.hold_ns ??
					(readout.closedAt !== null && readout.openedAt !== null
						? (Date.parse(readout.closedAt) - Date.parse(readout.openedAt)) *
							1e6
						: 0),
				expected_hold_ns: null,
				triggers: settle.triggers ?? ["desk settle"],
			}
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
			: readout.returnPct === null
				? `closed${exitTickText}`
				: `return ${(readout.returnPct * 100).toFixed(2)}%${exitTickText}`;

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "closed",
		opened_at: readout.openedAt,
		closed_at: readout.closedAt,
		book: row.virtual ? "virtual" : "paper",
		desk: true,
		policy_epoch: policyEpochFromRow(row),
		path: entryPath ?? (row.path as string | undefined),
		marks_a: tape?.a ?? null,
		marks_b: tape?.b ?? null,
		marks_c: tape?.c ?? null,
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
		outcome: outcome as LifeOutcome | null,
	};
};
