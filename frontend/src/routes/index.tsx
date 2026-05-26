import { createFileRoute } from "@tanstack/react-router";

import { Gauges } from "#/components/gauges";
import { ForceGraph } from "#/components/symm/ForceGraph";
import { TradeChart } from "#/components/symm/TradeChart";
import { Flex } from "#/components/ui/flex";

const TradingDashboard = () => {
	return (
		<Flex.Row gap={2} fullHeight fullWidth>
			<Flex.Column gap={2} padding={2} fullHeight fullWidth>
				<Flex.Row gap={2} fullWidth fullHeight>
					<Gauges />
				</Flex.Row>
				<Flex.Column fullWidth fullHeight>
					<TradeChart symbol="BTC/EUR" />
				</Flex.Column>
			</Flex.Column>
			<ForceGraph symbol="BTC/EUR" />
		</Flex.Row>
	);
};

export const Route = createFileRoute("/")({
	ssr: false,
	component: TradingDashboard,
});
