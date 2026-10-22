import { useSelector } from "@tanstack/react-store";
import { readPositionRows, signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import { openLots } from "#/components/learning/open-positions";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

/*
Count reads the open lots on the book selected in the top bar, the same
lots the open-positions panel draws.
*/
export const Count = () => {
	const book = useSelector(bookAtom, (state) => state);
	const store = signals["positions"];
	const rings = useSelector(store, (state) => state);
	const positions = readPositionRows(rings);
	const count = openLots(positions, book).length;

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
