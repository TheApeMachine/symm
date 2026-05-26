import "@tanstack/react-start/client-only";

import { Gauges } from "#/components/gauges";
import { TradesPanel } from "#/components/trades";
import { DashboardHeader } from "#/components/header";
import { ForceGraph } from "#/components/symm/ForceGraph";
import { TradeChart } from "#/components/symm/TradeChart";
import { Flex } from "#/components/ui/flex";
import { FluidSurfaceChart } from "#/components/symm/FluidSurfaceChart";
import { PredictionChart } from "#/components/symm/PredictionChart";
import {
	useSymmConnected,
	useSymmEnginePulse,
} from "#/lib/symm/use-dashboard-data";

export const TradingDashboard = () => {
	const connected = useSymmConnected();
	const pulse = useSymmEnginePulse();

	return (
		<>
			<DashboardHeader />
			<main className="min-h-0 overflow-hidden p-2">
				<Flex.Row gap={2} fullHeight fullWidth className="min-h-0">
					<Flex.Column gap={2} className="min-h-0 min-w-0 flex-[7]" fullHeight>
						<Flex.Row gap={2} className="min-h-[280px] shrink-0" fullWidth>
							<PredictionChart />
							<Flex.Column className="min-h-0 min-w-0 flex-[1.1]" fullHeight>
								<Gauges />
							</Flex.Column>
						</Flex.Row>
						<Flex.Column className="min-h-0 flex-1" fullWidth fullHeight>
							<TradeChart symbol="BTC/EUR" />
						</Flex.Column>
					</Flex.Column>
					<Flex.Column gap={2} className="min-h-0 min-w-0 flex-[3]" fullHeight>
						<FluidSurfaceChart />
						<ForceGraph symbol="BTC/EUR" className="min-h-0 flex-1" />
					</Flex.Column>
				</Flex.Row>
			</main>
			<aside className="min-h-0 overflow-hidden border-l border-(--dash-border) bg-(--dash-panel)">
				<TradesPanel />
			</aside>
			<footer className="flex h-8 shrink-0 items-center border-t border-(--dash-border) px-3 text-[10px] text-(--dash-muted)">
				<span>{connected ? "live" : "offline"}</span>
				<span className="mx-2">·</span>
				<span>measurements {pulse?.measurements ?? 0}</span>
				<span className="mx-2">·</span>
				<span>open {pulse?.open ?? 0}</span>
			</footer>
		</>
	);
};
