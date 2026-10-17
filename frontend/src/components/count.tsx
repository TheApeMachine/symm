import { useSelector } from "@tanstack/react-store";
import { bookAtom } from "#/collections/book";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { selectBook, useLifecycles } from "#/lib/lifecycle";

/*
Count reads the open-lot tally for the book selected in the top bar.
*/
export const Count = () => {
	const book = useSelector(bookAtom, (state) => state);
	const { report } = useLifecycles();
	const count = selectBook(report, book).open_count;

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
