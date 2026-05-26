import { Suspense, lazy } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";

import { DashboardHeader } from "#/components/header";
import { TradesPanel } from "#/components/trades";
import {
	useSymmConnected,
	useSymmEnginePulse,
} from "#/lib/symm/use-dashboard-data";

const PredictionChart = lazy(() =>
	import("#/components/symm/PredictionChart").then((module) => ({
		default: module.PredictionChart,
	})),
);

const Gauges = lazy(() =>
	import("#/components/gauges").then((module) => ({
		default: module.Gauges,
	})),
);

const TradeChart = lazy(() =>
	import("#/components/symm/TradeChart").then((module) => ({
		default: module.TradeChart,
	})),
);

const FluidSurfaceChart = lazy(() =>
	import("#/components/symm/FluidSurfaceChart").then((module) => ({
		default: module.FluidSurfaceChart,
	})),
);

const ForceGraph = lazy(() =>
	import("#/components/symm/ForceGraph").then((module) => ({
		default: module.ForceGraph,
	})),
);

const ChartFallback = () => (
	<div className="flex min-h-0 flex-1 items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) text-xs text-(--dash-muted)">
		Loading chart…
	</div>
);

const TradingDashboard = () => {
	const connected = useSymmConnected();
	const pulse = useSymmEnginePulse();

	return (
		<div className="dashboard-shell">
			<DashboardHeader />
			<main className="dashboard-main">
				<div className="dashboard-workspace">
					<section className="dashboard-primary">
						<div className="dashboard-top-row">
							<Suspense fallback={<ChartFallback />}>
								<PredictionChart />
							</Suspense>
							<Suspense fallback={<ChartFallback />}>
								<Gauges />
							</Suspense>
						</div>
						<div className="dashboard-trade-panel">
							<div className="dashboard-panel-header">BTC/EUR</div>
							<Suspense fallback={<ChartFallback />}>
								<TradeChart symbol="BTC/EUR" />
							</Suspense>
						</div>
					</section>
					<section className="dashboard-secondary">
						<div className="dashboard-trades-strip">
							<TradesPanel />
						</div>
						<Suspense fallback={<ChartFallback />}>
							<FluidSurfaceChart />
						</Suspense>
						<Suspense fallback={<ChartFallback />}>
							<ForceGraph symbol="BTC/EUR" className="min-h-0" />
						</Suspense>
					</section>
				</div>
			</main>
			<footer className="dashboard-footer">
				<span>{connected ? "live" : "offline"}</span>
				<span className="mx-2">·</span>
				<span>measurements {pulse?.measurements ?? 0}</span>
				<span className="mx-2">·</span>
				<span>open {pulse?.open ?? 0}</span>
			</footer>
		</div>
	);
};

export const Route = createLazyFileRoute("/")({
	component: TradingDashboard,
});
