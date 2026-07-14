import { createStore } from "@tanstack/react-store";
import { tradeObservationKey } from "#/collections/snapshot-retain";
import type { TradeObservation } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

const TRADE_JOURNAL_CAPACITY = 256;

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
tradeJournalValues returns retained observations oldest-first so journal and
audit surfaces can scan immutable broker history without cloning the buffer.
*/
export const tradeJournalValues = (
	buffer: CircularBuffer<TradeObservation>,
): TradeObservation[] => buffer.values();

/*
pushJournalObservations appends only unseen rows so cumulative thesis snapshots
and coalesced websocket batches cannot replace history with a shorter replay.
*/
export const pushJournalObservations = (
	buffer: CircularBuffer<TradeObservation>,
	incoming: TradeObservation[],
): { journal: CircularBuffer<TradeObservation>; appended: boolean } => {
	if (incoming.length === 0) {
		return { journal: buffer, appended: false };
	}

	const knownKeys = new Set(buffer.values().map(tradeObservationKey));
	const appendedRows: TradeObservation[] = [];

	for (const observation of incoming) {
		const key = tradeObservationKey(observation);

		if (knownKeys.has(key)) {
			continue;
		}

		appendedRows.push(observation);
		knownKeys.add(key);
	}

	if (appendedRows.length === 0) {
		return { journal: buffer, appended: false };
	}

	const journal = Circular<TradeObservation>(buffer.capacity());

	for (const observation of buffer.values()) {
		journal.push(observation);
	}

	for (const observation of appendedRows) {
		journal.push(observation);
	}

	return { journal, appended: true };
};

/*
tradeJournalStore retains immutable broker facts in publication order inside one
Circular buffer so later thesis snapshots append history instead of replacing it.
*/
export const tradeJournalStore = createStore(
	{
		journal: Circular<TradeObservation>(TRADE_JOURNAL_CAPACITY),
		version: 0,
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const incoming = asJournal(frame);

				if (incoming.length === 0) {
					return {
						...prev,
						observed: true,
					};
				}

				const { journal, appended } = pushJournalObservations(
					prev.journal,
					incoming,
				);

				if (!appended) {
					return {
						...prev,
						observed: true,
					};
				}

				return {
					journal,
					version: prev.version + 1,
					observed: true,
				};
			}),
		reset: () =>
			setState(() => ({
				journal: Circular<TradeObservation>(TRADE_JOURNAL_CAPACITY),
				version: 0,
				observed: false,
			})),
	}),
);
