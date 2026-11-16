import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import { OpenChoiceChart } from "#/components/learning/open-choice-chart";
import { Flex } from "#/components/ui";
import { comparePositionRows } from "#/lib/frame-position";

export const positionRows = (state: Record<string, Position>): Position[] => {
	const rows = Object.values(state);
	rows.sort(comparePositionRows);
	return rows;
};

export const positionsRevision = (state: Record<string, Position>): string => {
	const parts = Object.keys(state);
	parts.sort();
	return parts.join("|");
};

export const openPositions = (rows: Position[], book: Book) =>
	rows
		.filter((row) => {
			const tier = row.tier_key && row.tier_key !== "" ? row.tier_key : "paper";
			return !row.closed_at && tier === book;
		})
		.map((row) => row.symbol)
		.sort();

export const OpenPositions = () => {
	const book = useSelector(bookAtom, (state) => state);
	const watching = useSelector(signals.positions, (state) => {
		for (const pos of Object.values(state as Record<string, Position>)) {
			const tier = pos.tier_key && pos.tier_key !== "" ? pos.tier_key : "paper";
			if (!pos.closed_at && tier === book) return true;
		}
		return false;
	});

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<OpenChoiceChart />
			{!watching ? (
				<Flex.Column
					align="center"
					justify="center"
					gap={2}
					className="absolute inset-0 z-10 min-h-48 bg-(--surface) px-4 text-center"
				>
					<span className="font-mono text-[11px] text-(--f4) tracking-[0.14em] uppercase">
						no open positions
					</span>
					<span className="max-w-sm text-(--f3) text-[12px] leading-relaxed">
						Held positions on this book paint here as they mark.
					</span>
				</Flex.Column>
			) : null}
		</div>
	);
};
