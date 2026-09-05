import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

/*
Count reads the open-lot tally off the positions store and states it in the same
label-over-value form the cash readings use, so the top bar's right-hand side is
one row of readouts rather than a sentence sitting among them.
*/
export const Count = () => {
	const last = useSelector(positionStore, (state) =>
		state.findLast(() => true),
	);

	return (
		<Flex.Column className="items-end gap-px">
			<Typography.Label size="s" tone="f4" weight="normal">
				Positions
			</Typography.Label>
			<Typography.Mono size="lg" tone="f1" weight="medium" data-count>
				{String(last ? last.rowsLength() : 0)}
			</Typography.Mono>
		</Flex.Column>
	);
};
