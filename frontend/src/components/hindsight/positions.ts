import type { HindsightLifecycleEvent } from "./hindsight-types";

/*
Positions the desk actually held, assembled from the recorded lifecycle tape.

Every number here is a venue fact: the fill price the exchange reported, the
quantity it filled, the fee it charged. Nothing is modelled, assumed, or
reconstructed — where the record is silent the field stays undefined rather
than being filled with a plausible value.

A position is keyed by the decision that caused it, because that is how the
lifecycle tape is correlated: a transition happens inside the broker after the
planner committed a decision, and the decision names it. The instants are used
only to place the position on a time-shaped axis; they never establish which
records belong together.
*/

export type PositionFill = {
	at: string;
	price: number | null;
	quantity: number | null;
	fee: number | null;
	orderId: string;
	execId: string;
	side: string;
	status: string;
};

export type Position = {
	decisionId: string;
	symbol: string;
	entry: PositionFill | null;
	exit: PositionFill | null;
	openedAt: string | null;
	closedAt: string | null;
	/* Still open at the end of the recorded tape. */
	open: boolean;
	/*
		The realised change between the two fill prices, as a fraction of the
		entry fill. Defined only when both fills were recorded. It is the price
		the desk actually transacted at on both sides — not a market excursion,
		and not a return net of anything the record does not state.
	*/
	realisedPriceChange: number | null;
	/* Fees the venue actually charged across both fills, where reported. */
	fees: number | null;
};

const numberOrNull = (raw?: string | null): number | null => {
	if (raw === undefined || raw === null || raw === "") return null;

	const value = Number(raw);

	return Number.isFinite(value) ? value : null;
};

const fillOf = (event: HindsightLifecycleEvent): PositionFill | null => {
	const execution = event.execution;

	if (execution === undefined || execution === null) return null;

	return {
		at: execution.fillAt || event.at,
		price: numberOrNull(execution.avgPrice) ?? numberOrNull(execution.lastPrice),
		quantity: numberOrNull(execution.cumQty) ?? numberOrNull(execution.lastQty),
		fee: numberOrNull(execution.feeUsdEquiv),
		orderId: execution.orderId ?? "",
		execId: execution.execId ?? "",
		side: execution.side ?? "",
		status: execution.orderStatus ?? "",
	};
};

/*
buildPositions folds the lifecycle tape into one record per decision, in the
order the decisions first appear. An entry with no recorded exit stays open
rather than being closed at the end of the tape.
*/
export const buildPositions = (
	events: HindsightLifecycleEvent[],
): Position[] => {
	const byDecision = new Map<string, Position>();

	for (const event of events) {
		if (event.decisionId === "") continue;

		const existing = byDecision.get(event.decisionId) ?? {
			decisionId: event.decisionId,
			symbol: event.symbol,
			entry: null,
			exit: null,
			openedAt: null,
			closedAt: null,
			open: true,
			realisedPriceChange: null,
			fees: null,
		};

		switch (event.kind) {
			case "entry_fill":
				existing.entry = fillOf(event) ?? existing.entry;
				break;
			case "exit_fill":
				existing.exit = fillOf(event) ?? existing.exit;
				break;
			case "position_open":
				existing.openedAt = event.at;
				break;
			case "position_close":
				existing.closedAt = event.at;
				existing.open = false;
				break;
			default:
				break;
		}

		if (existing.symbol === "") existing.symbol = event.symbol;

		byDecision.set(event.decisionId, existing);
	}

	for (const position of byDecision.values()) {
		const entry = position.entry?.price ?? null;
		const exit = position.exit?.price ?? null;

		if (entry !== null && exit !== null && entry > 0) {
			position.realisedPriceChange = (exit - entry) / entry;
		}

		const entryFee = position.entry?.fee;
		const exitFee = position.exit?.fee;

		if (entryFee != null || exitFee != null) {
			position.fees = (entryFee ?? 0) + (exitFee ?? 0);
		}

		if (position.exit !== null) position.open = false;
	}

	return [...byDecision.values()];
};

/*
positionsFor narrows the desk's record to one instrument, which is what the
per-symbol chart can honestly draw. The count of everything else stays
available so a reader is never left thinking the desk did nothing elsewhere.
*/
export const positionsFor = (
	positions: Position[],
	symbol: string,
): Position[] =>
	positions.filter((position) => position.symbol === symbol);

/*
positionInstant is the instant a position edge sits at on a time axis: the
fill's own reported instant where the venue gave one, otherwise the lifecycle
transition's instant.
*/
export const positionInstant = (fill: PositionFill | null): number | null => {
	if (fill === null) return null;

	const at = new Date(fill.at).getTime();

	return Number.isNaN(at) ? null : at;
};
