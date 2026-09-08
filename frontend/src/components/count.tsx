import { learningStore } from "#/collections/learning";
import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

/*
Count reads the learning wallet when present, otherwise the account positions,
and states the open-lot tally in the same
label-over-value form the cash readings use, so the top bar's right-hand side is
one row of readouts rather than a sentence sitting among them.
*/
export const Count = () => {
	const policy = useSelector(learningStore, (state) => state?.agents[0]);
	const last = useSelector(positionStore, (state) =>
		state.findLast(() => true),
	);

	return (
		<Flex.Column className="items-end gap-px">
			<Typography.Label size="s" tone="f4" weight="normal">
				Positions
			</Typography.Label>
			<Typography.Mono
				size="lg"
				tone={policy ? "f3" : "f1"}
				weight="medium"
				data-count
			>
				{String(
					policy
						? policy.positions.filter(
								(position) => Number(position.holding?.qty) > 0,
							).length
						: last
							? last.rowsLength()
							: 0,
				)}
			</Typography.Mono>
		</Flex.Column>
	);
};
