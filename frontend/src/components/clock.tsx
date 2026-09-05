import { useSelector } from "@tanstack/react-store";
import { strategyStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";

const fmtTime = (instant: Date): string => instant.toISOString().slice(11, 19);
const fmtDate = (instant: Date): string => instant.toISOString().slice(0, 10);

export const Clock = () => {
	const last = useSelector(strategyStore, (state) => state.getLast());

	const decisionsLen = last ? last.decisionsLength() : 0;
	let instant: Date | null = null;

	if (decisionsLen > 0) {
		const dec = last?.decisions(0);
		const at = dec?.at();
		if (at) {
			instant = new Date(Number(at));
		}
	}

	if (!instant || !Number.isFinite(instant.getTime())) {
		instant = new Date();
	}

	return (
		<Flex.Column>
			<Flex>
				<span data-time>{`${fmtTime(instant)} UTC`}</span>
			</Flex>
			<Flex className="text-(--f4)">
				<span data-date>{`${fmtDate(instant)} engine clock`}</span>
			</Flex>
		</Flex.Column>
	);
};
