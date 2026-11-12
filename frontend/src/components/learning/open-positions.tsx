import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import { isClosed, onTier, pnlFrac } from "#/components/book-readout";
import { OpenChoiceChart } from "#/components/learning/open-choice-chart";
import { chartHeld } from "#/components/learning/open-choice-draw";
import { Flex } from "#/components/ui";
import { comparePositionRows } from "#/lib/frame-position";

/*
Open positions are the desk's per-lot records on signals.positions.
*/
export const positionRows = (state: Record<string, Position>): Position[] => {
	const rows = Object.values(state);
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
export const chartHeldRow = (pos: Position, book: Book) =>
	onTier(pos, book) && !isClosed(pos);

export const openLots = (rows: Position[], book: Book) =>
	rows
		.filter((row) => !isClosed(row) && onTier(row, book))
		.map((row) => row.symbol)
		.sort();

/** Close-now return after fees. */
export const lotInProfit = (pos: Position) => {
	const pct = pnlFrac(pos);
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
				className="h-full min-h-48 px-4 text-center"
			>
				<span className="font-mono text-[11px] text-(--f4) tracking-[0.14em] uppercase">
					no open lots
				</span>
				<span className="max-w-sm text-(--f3) text-[12px] leading-relaxed">
					Held positions on this book paint here as they mark.
				</span>
			</Flex.Column>
		);
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<OpenChoiceChart />
		</div>
	);
};

const hasHeld = (state: Record<string, Position>, book: Book) => {
	for (const pos of Object.values(state)) {
		if (chartHeld(pos, book)) return true;
	}
	return false;
};
