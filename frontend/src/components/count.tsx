import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import { openLots, positionRows } from "#/components/learning/open-positions";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

/*
Count reads the open-lot cardinality for the selected book. The selector
returns a number so mark-only position frames do not re-render the top bar.
*/
export const Count = () => {
	const book = useSelector(bookAtom, (state) => state);
	const count = useSelector(signals.positions, (state) =>
		openLots(positionRows(state as Record<string, Position>), book).length,
	);

	return (
		<Flex.Column align="end" gap={1}>
			<Typography.Label size="s" tone="f4" weight="normal">
				Positions
			</Typography.Label>
			<Typography.Mono size="lg" tone="f1" weight="medium" data-count>
				{String(count ?? 0)}
			</Typography.Mono>
		</Flex.Column>
	);
};
