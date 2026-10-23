import { useSelector } from "@tanstack/react-store";
import { bookAtom } from "#/collections/book";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { ratePercent, selectBook, useLifecycles } from "#/lib/lifecycle";

/*
Top-bar win rate and edge match the Position Life-Cycle header: settled desk
round trips the session remembers, including virtual closes Train already
dropped from the live positions snapshot.
*/
export const PaperPerformance = () => {
	const book = useSelector(bookAtom, (state) => state);
	const { report } = useLifecycles();
	const perf = selectBook(report, book).performance;
	const name = book === "virtual" ? "virtual" : "paper";
	const title =
		perf.closed > 0
			? `${name} settled lots · win rate ${ratePercent(perf.win_rate)} · mean edge ${ratePercent(perf.mean_return)} (${perf.closed} closed)`
			: `no settled ${name} round trips this session yet`;

	return (
		<Flex.Row align="center" gap={6} data-wallet="performance" title={title}>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Win rate
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone="f2"
					weight="medium"
					data-performance="win-rate"
				>
					{ratePercent(perf.win_rate)}
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Edge / trade
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone="f2"
					weight="medium"
					data-performance="edge"
				>
					{ratePercent(perf.mean_return)}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
