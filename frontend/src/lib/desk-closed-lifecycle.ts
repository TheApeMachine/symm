import {
	decimal,
	positionReadout,
	settledOutcome,
} from "#/components/book-readout";
import type { Lifecycle, LifeOutcome } from "#/lib/lifecycle";
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

	return {
		id: row.id as string,
		symbol: row.symbol as string,
		status: "closed",
		opened_at: readout.openedAt,
		closed_at: readout.closedAt,
		book: row.virtual ? "virtual" : "paper",
		desk: true,
		policy_epoch: row.run as string | undefined,
		path: row.path as string | undefined,
		marks_a: (row.marks as Record<string, unknown>)?.a as number | null,
		marks_b: (row.marks as Record<string, unknown>)?.b as number | null,
		marks_c: (row.marks as Record<string, unknown>)?.c as number | null,
		events: [
			{
				at: readout.openedAt,
				kind: "opened",
				detail: "restored from stored desk lot",
			},
			{
				at: readout.closedAt,
				kind: "closed",
				detail:
					readout.returnPct === null
						? "closed"
						: `return ${(readout.returnPct * 100).toFixed(2)}%`,
			},
		],
		outcome: outcome as LifeOutcome | null,
	};
};
