import { useSelector } from "@tanstack/react-store";
import { bookAtom } from "#/collections/book";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { NOT_AVAILABLE, ratePercent, selectBook, useLifecycles } from "#/lib/lifecycle";

/*
PaperPerformance is the top bar's reading of closed paper trades: win rate and
mean return per closed paper round trip. Paper trading has no real-exchange
ledger: the title says so. Dashes until a trade has closed.
*/
export const PaperPerformance = () => {
	const book = useSelector(bookAtom, (state) => state);
	const { report } = useLifecycles();
	const perf = selectBook(report, book).performance;
	const name = book === "virtual" ? "virtual" : "paper";
	const title =
		perf.closed > 0
			? [
					`${perf.closed} closed`,
					`${perf.wins}/${perf.closed} ${name} wins (${ratePercent(perf.win_rate)}), mean edge ${ratePercent(perf.mean_return)}, P&L ${perf.pnl.toFixed(2)}`,
					`exchange: ${NOT_AVAILABLE}`,
				].join("\n")
			: `no closed ${name} trades yet`;

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
