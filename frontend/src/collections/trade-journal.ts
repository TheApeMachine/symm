import { createStore } from "@tanstack/react-store";
import type { TradeObservation } from "#/types/thesis";

const asJournal = (frame: unknown): TradeObservation[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is TradeObservation =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as TradeObservation).symbol === "string" &&
			typeof (row as TradeObservation).kind === "string",
	);
};

/*
tradeJournalStore retains the backend thesis.TradeJournal snapshot in publication
order so the journal surface can render one immutable broker and position trail.
*/
export const tradeJournalStore = createStore(
	{
		observations: [] as TradeObservation[],
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState(() => ({
				observations: asJournal(frame),
			})),
		reset: () =>
			setState(() => ({
				observations: [],
			})),
	}),
);
