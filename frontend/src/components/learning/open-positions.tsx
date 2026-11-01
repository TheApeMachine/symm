import { useSelector } from "@tanstack/react-store";
import { useLayoutEffect, useMemo, useState } from "react";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import { onBook, positionReadout } from "#/components/book-readout";
import { OpenChoiceChart } from "#/components/learning/open-choice-chart";
import {
	openChoiceLotsFromRows,
	syncOpenChoiceLots,
} from "#/components/learning/open-choice-trail";
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
	const positionsState = useSelector(signals.positions, (state) => state);
	const held = useMemo(
		() =>
			positionRows(positionsState)
				.filter((row) => !positionReadout(row).closed && onBook(row, book))
				.sort(comparePositionRows),
		[book, positionsState],
	);

	const [trailGen, setTrailGen] = useState(0);

	useLayoutEffect(() => {
		if (syncOpenChoiceLots(openChoiceLotsFromRows(held))) {
			setTrailGen((gen) => gen + 1);
		}
	}, [held]);

	if (held.length === 0) {
		return (
			<Flex.Column
				align="center"
				justify="center"
				gap={2}
				className="flex-1 text-(--f3)"
			>
				<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
				<span className="text-sm">
					{book === "virtual" ? "No virtual trades" : "No open positions"}
				</span>
				<span className="text-[10px] text-(--f4)">
					{book === "virtual"
						? "a chart appears when a finished rule opens a virtual trade"
						: "the chart fills as soon as the desk holds a symbol"}
				</span>
			</Flex.Column>
		);
	}

	return (
		<Flex.Column className="h-full min-h-0 w-full min-w-0 flex-1 overflow-hidden">
			<OpenChoiceChart lots={held} trailGen={trailGen} />
		</Flex.Column>
	);
};
