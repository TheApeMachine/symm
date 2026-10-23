import { useSelector } from "@tanstack/react-store";
import { clockAtom } from "#/collections/app";
import { Flex } from "#/components/ui/flex";

const fmtTime = (instant: Date): string => instant.toISOString().slice(11, 19);
const fmtDate = (instant: Date): string => instant.toISOString().slice(0, 10);

export const Clock = () => {
	const timestamp = useSelector(clockAtom, (state) => state);
	const instant =
		typeof timestamp === "number" && Number.isFinite(timestamp)
			? new Date(timestamp)
			: new Date();

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
