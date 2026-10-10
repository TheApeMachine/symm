import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { ratePercent, useLifecycles } from "#/lib/lifecycle";

/*
PaperPerformance is the top bar's reading of closed paper trades: win rate and
mean return per trade on the shadow ledger (the evaluation truth), with the
venue's beside it in the title. Dashes until a trade has closed.
*/
export const PaperPerformance = () => {
	const { report } = useLifecycles();
	const perf = report?.performance;
	const title = perf
		? [
				`${perf.closed} closed`,
				`shadow: ${perf.shadow_wins}/${perf.shadow_trades} wins, mean ${ratePercent(perf.shadow_mean_return)}, P&L ${perf.shadow_pnl.toFixed(2)}`,
				`venue: ${perf.venue_wins} wins (${ratePercent(perf.venue_win_rate)}), mean ${ratePercent(perf.venue_mean_return)}, P&L ${perf.venue_pnl.toFixed(2)}`,
			].join("\n")
		: "no closed paper trades yet";

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
					{ratePercent(perf?.shadow_win_rate)}
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
					{ratePercent(perf?.shadow_mean_return)}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
