import { useSelector } from "@tanstack/react-store";
import { positionCountStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

/*
Count reads the account positions and states the open-lot tally in the same
label-over-value form the cash readings use, so the top bar's right-hand side is
one row of readouts rather than a sentence sitting among them.
*/
export const Count = () => {
	const count = useSelector(positionCountStore, (state) => state);

	return (
		<Flex.Column className="items-end gap-px">
			<Typography.Label size="s" tone="f4" weight="normal">
				Positions
			</Typography.Label>
			<Typography.Mono
				size="lg"
				tone="f1"
				weight="medium"
				data-count
			>
				{String(count ?? 0)}
			</Typography.Mono>
		</Flex.Column>
	);
};
