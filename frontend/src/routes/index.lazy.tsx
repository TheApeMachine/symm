import { Suspense, lazy, useMemo } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";

import { DashboardHeader } from "#/components/header";
import { TradesPanel } from "#/components/trades";
import {
	useSymmConnected,
	useSymmEnginePulse,
	useSymmWallet,
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

const TradeChartGrid = lazy(() =>
	import("#/components/symm/TradeChart").then((module) => ({
		default: module.TradeChartGrid,
	})),
);

const FluidSurfaceChart = lazy(() =>
	import("#/components/symm/FluidSurfaceChart").then((module) => ({
		default: module.FluidSurfaceChart,
	})),
);

const ChartFallback = () => (
	<div className="flex min-h-0 flex-1 items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) text-xs text-(--dash-muted)">
		Loading chart…
	</div>
);

const useOpenChartSymbols = () => {
	const wallet = useSymmWallet();

	return useMemo(() => {
		const currency = wallet.currency || "EUR";
		const open = Object.entries(wallet.inventory)
			.filter(([, qty]) => qty > 0)
			.map(([base]) => `${base}/${currency}`);

		if (open.length > 0) {
			return open;
		}

		return ["BTC/EUR"];
	}, [wallet.currency, wallet.inventory]);
};

const TradingDashboard = () => {
	const connected = useSymmConnected();
	const pulse = useSymmEnginePulse();
	const chartSymbols = useOpenChartSymbols();

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
							<Suspense fallback={<ChartFallback />}>
								<TradeChartGrid symbols={chartSymbols} />
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
