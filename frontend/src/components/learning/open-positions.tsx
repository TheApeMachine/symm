import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import { onBook, positionReadout } from "#/components/book-readout";
import { OpenChoiceChart } from "#/components/learning/open-choice-chart";
import { chartHeld } from "#/components/learning/open-choice-draw";
import { Flex } from "#/components/ui";
import { comparePositionRows } from "#/lib/frame-position";

/*
Open positions are the desk's per-lot records on signals.positions.
*/
export const positionRows = (
	state: Record<string, Position>,
): Record<string, unknown>[] => {
	const rows = Object.values(state).map(
		(row) => row as unknown as Record<string, unknown>,
	);
	rows.sort(comparePositionRows);
	return rows;
};

/** Cheap fingerprint of position-store ids. */
export const positionsRevision = (state: Record<string, Position>): string => {
	const parts = Object.keys(state);
	parts.sort();
	return parts.join("|");
};

/** Open lots on the selected book. */
export const chartHeldRow = (row: Record<string, unknown>, book: Book) => {
	if (!onBook(row, book)) return false;
	return !positionReadout(row).closed;
};

export const openLots = (rows: Record<string, unknown>[], book: Book) =>
	rows
		.filter((row) => !positionReadout(row).closed && onBook(row, book))
		.map((row) => row.symbol)
		.sort();

/** Close-now return after fees. */
export const lotInProfit = (row: Record<string, unknown>) => {
	const pct = positionReadout(row).returnPct;
	return pct !== null && pct > 0;
};

export const OpenPositions = () => {
	const book = useSelector(bookAtom, (state) => state);
	const watching = useSelector(signals.positions, (state) =>
		hasHeld(state as Record<string, Position>, book),
	);

	if (!watching) {
		return (
			<Flex.Column
				align="center"
				justify="center"
				gap={2}
				className="flex-1 text-(--f3)"
			>
				<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
				<span className="text-sm">
					{book === "virtual"
						? "No virtual trades"
						: book === "real"
							? "No real trades"
							: "No open positions"}
				</span>
				<span className="text-[10px] text-(--f4)">
					{book === "virtual"
						? "a chart appears when a finished rule opens a virtual trade"
						: book === "real"
							? "live lots appear once paper edge arms real trading"
							: "the chart fills as soon as the desk holds a symbol"}
				</span>
			</Flex.Column>
		);
	}

	return (
		<Flex.Column className="h-full min-h-0 w-full min-w-0 flex-1 overflow-hidden">
			<OpenChoiceChart />
		</Flex.Column>
	);
};

const hasHeld = (state: Record<string, Position>, book: Book) => {
	for (const pos of Object.values(state)) {
		if (chartHeld(pos, book)) return true;
	}
	return false;
};
