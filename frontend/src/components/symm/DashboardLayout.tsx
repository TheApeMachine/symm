import { Suspense, lazy, useMemo } from "react";

import { AuditPanel } from "#/components/audit";
import { Gauges } from "#/components/gauges";
import { PanelTabs } from "#/components/symm/PanelTabs";
import { SignalHeatmap } from "#/components/symm/SignalHeatmap";
import { TradesPanel } from "#/components/trades";
import { panelsByType, type LayoutPanel } from "#/lib/symm/layout-schema";
import { useSymmWallet } from "#/lib/symm/use-dashboard-data";
import { useDashboardLayout } from "#/lib/symm/use-dashboard-layout";

const PredictionChart = lazy(() =>
	import("#/components/symm/PredictionChart").then((module) => ({
		default: module.PredictionChart,
	})),
);

const TradeChartGrid = lazy(() =>
	import("#/components/symm/TradeChart").then((module) => ({
		default: module.TradeChartGrid,
	})),
);

const GenericSurfaceChart = lazy(() =>
	import("#/components/symm/GenericSurfaceChart").then((module) => ({
		default: module.GenericSurfaceChart,
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

const SurfacePanel = ({ panel }: { panel: LayoutPanel }) => (
	<div className="dashboard-fluid-chart">
		<Suspense fallback={<ChartFallback />}>
			<GenericSurfaceChart panel={panel} />
		</Suspense>
	</div>
);

export const DashboardLayout = () => {
	const layout = useDashboardLayout();
	const chartSymbols = useOpenChartSymbols();
	const gaugePanel = panelsByType(layout, "gauge_grid")[0];
	const surfacePanel = panelsByType(layout, "surface")[0];

	const hasPrediction = panelsByType(layout, "prediction_chart").length > 0;
	const hasTradeGrid = panelsByType(layout, "trade_grid").length > 0;
	const hasTradesPanel = panelsByType(layout, "trades_panel").length > 0;
	const hasAuditPanel = panelsByType(layout, "audit_panel").length > 0;

	return (
		<div className="dashboard-workspace">
			<section className="dashboard-primary">
				<div className="dashboard-top-row">
					{/* Top-left: Prediction chart OR signal heatmap */}
					{hasPrediction ? (
						<PanelTabs
							id="top-left"
							className="rounded border border-(--dash-border) bg-(--dash-panel)"
							tabs={[
								{
									key: "prediction",
									label: "Prediction",
									content: (
										<Suspense fallback={<ChartFallback />}>
											<PredictionChart />
										</Suspense>
									),
								},
								{
									key: "heatmap",
									label: "Signal Map",
									content: <SignalHeatmap />,
								},
							]}
						/>
					) : null}

					{/* Top-right: Gauges */}
					{gaugePanel !== undefined ? <Gauges panel={gaugePanel} /> : null}
				</div>

				{/* Main chart area: OHLC trade charts OR signal heatmap full-width */}
				{hasTradeGrid ? (
					<PanelTabs
						id="main-chart"
						className="dashboard-trade-panel"
						tabs={[
							{
								key: "candles",
								label: "Charts",
								content: (
									<div className="flex min-h-0 flex-1 overflow-hidden p-1">
										<Suspense fallback={<ChartFallback />}>
											<TradeChartGrid symbols={chartSymbols} />
										</Suspense>
									</div>
								),
							},
							{
								key: "heatmap",
								label: "Signal Map",
								content: (
									<div className="min-h-0 flex-1 p-1">
										<SignalHeatmap />
									</div>
								),
							},
						]}
					/>
				) : null}
			</section>

			<section className="dashboard-secondary">
				{/* Sidebar: trades + audit with tabs */}
				<div className="dashboard-trades-strip">
					{hasTradesPanel ? <TradesPanel /> : null}
					{hasAuditPanel ? <AuditPanel /> : null}
				</div>

				{/* Bottom: fluid 3D surface */}
				{surfacePanel !== undefined ? (
					<SurfacePanel panel={surfacePanel} />
				) : null}
			</section>
		</div>
	);
};
