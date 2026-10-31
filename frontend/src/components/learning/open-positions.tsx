import { useSelector } from "@tanstack/react-store";
import { useLayoutEffect, useMemo, useState } from "react";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import {
	comparePositionRows,
	framePositionRows,
} from "#/collections/positions";
import { onBook, positionReadout } from "#/components/book-readout";
import { OpenChoiceChart } from "#/components/learning/open-choice-chart";
import {
	openChoiceLotsFromRows,
	syncOpenChoiceLots,
} from "#/components/learning/open-choice-trail";
import { Flex } from "#/components/ui";

/*
Open positions are the desk's per-lot frames. The connect snapshot and each
later mark tick land on the positions store; a tick names only the lots
that moved, and the book keeps every lot that tick did not mention.
*/

/*
positionRows decodes every lot frame the store holds into its row objects.
The "positions" layer is a JSON array; each element is one lot row.
A layer that decodes to a single object (not an array) is also accepted.

A connect snapshot and a later per-lot update both carry the same id. The
newer tick wins, and a lot that only appears in one of them stays.
*/
export const positionRows = (
	state: Record<
		string,
		{ lookup: (keys: string[]) => unknown[]; tick?: bigint }
	>,
): Record<string, unknown>[] => {
	const byID = new Map<
		string,
		{ row: Record<string, unknown>; tick: bigint }
	>();
	const anon: Record<string, unknown>[] = [];

	const take = (row: Record<string, unknown>, tick: bigint) => {
		const id = typeof row.id === "string" ? row.id : "";

		if (id === "") {
			anon.push(row);
			return;
		}

		const prior = byID.get(id);

		if (!prior || tick >= prior.tick) {
			byID.set(id, { row, tick });
		}
	};

	for (const frame of Object.values(state)) {
		const tick = frame.tick ?? 0n;

		for (const row of framePositionRows(frame)) {
			take(row, tick);
		}
	}

	const rows = [...Array.from(byID.values(), (held) => held.row), ...anon];
	rows.sort(comparePositionRows);
	return rows;
};

/** Cheap fingerprint of position-store ticks across scopes. */
export const positionsRevision = (
	state: Record<string, { tick?: bigint }>,
): string => {
	const parts: string[] = [];
	for (const [scope, frame] of Object.entries(state)) {
		parts.push(`${scope}:${frame.tick ?? 0n}`);
	}
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
