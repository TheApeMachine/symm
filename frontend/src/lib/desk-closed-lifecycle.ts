import type { DeskPosition } from "#/collections/types";
import { positionReadout } from "#/components/book-readout";
import type { Lifecycle } from "#/lib/lifecycle";

/*
Desk lifecycle rows are derived from stored order and execution facts. A row
without a complete close or stable identity is not a closed round trip.
*/
export const lifecycleFromDeskClosed = (
	row: DeskPosition,
): Lifecycle | null => {
	if (!row.id) return null;

	const readout = positionReadout(row);
	if (!readout.closed || readout.openedAt === null || readout.closedAt === null) {
		return null;
	}

	const outcome =
		readout.entryCost === null ||
		readout.exitProceeds === null ||
		readout.fees === null ||
		readout.netPnl === null ||
		readout.returnPct === null ||
		readout.holdNs === null
			? null
			: {
					cost: readout.entryCost,
					proceeds: readout.exitProceeds,
					fees: readout.fees,
					realized: readout.netPnl,
					net_return: readout.returnPct,
					hold_ns: readout.holdNs,
					expected_hold_ns: null,
					triggers: ["stored desk close"],
				};

	return {
		id: row.id,
		symbol: row.symbol,
		status: "closed",
		opened_at: readout.openedAt,
		closed_at: readout.closedAt,
		book: row.virtual ? "virtual" : "paper",
		policy_epoch: row.run ?? undefined,
		marks_a: row.marks?.a ?? null,
		marks_b: row.marks?.b ?? null,
		marks_c: row.marks?.c ?? null,
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
		outcome,
	};
};