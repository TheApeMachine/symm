import { lazy, Suspense, useMemo } from "react";

import { AuditPanel } from "#/components/audit";
import { Gauges } from "#/components/charts/confidence/Gauges";
import { SignalHeatmap } from "#/components/charts/confidence/SignalHeatmap";
import { SignalSurpriseHeatmap } from "#/components/charts/confidence/SignalSurpriseHeatmap";
import { PanelTabs } from "#/components/dashboard/PanelTabs";
import { TradesPanel } from "#/components/trades";
import {
	type LayoutPanel,
	mergedGaugePanel,
	panelsByType,
} from "#/lib/symm/layout-schema";
import { useSymmWallet } from "#/lib/symm/use-dashboard-data";
import { useDashboardLayout } from "#/lib/symm/use-dashboard-layout";

const PredictionChart = lazy(() =>
	import("#/components/charts/prediction/PredictionChart").then((module) => ({
		default: module.PredictionChart,
	})),
);

const TradeChartGrid = lazy(() =>
	import("#/components/charts/trade/TradeChart").then((module) => ({
		default: module.TradeChartGrid,
	})),
);

const SurfaceChart = lazy(() =>
	import("#/components/charts/fluid/SurfaceChart").then((module) => ({
		default: module.SurfaceChart,
	})),
);

const ChartFallback = () => (
	<div className="flex h-full min-h-0 flex-1 items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) text-xs text-(--dash-muted)">
		Loading chart…
	</div>
);

const useOpenChartSymbols = (anchorSymbol?: string) => {
	const wallet = useSymmWallet();

	return useMemo(() => {
		const currency = wallet.currency || "EUR";
		const open = Object.entries(wallet.inventory)
			.filter(([, qty]) => qty > 0)
			.map(([base]) => `${base}/${currency}`);

		if (open.length > 0) {
			return open;
		}

		if (anchorSymbol !== undefined && anchorSymbol.length > 0) {
			return [anchorSymbol];
		}

		return ["BTC/EUR"];
	}, [wallet.currency, wallet.inventory, anchorSymbol]);
};

const SurfacePanel = ({ panel }: { panel: LayoutPanel }) => (
	<div className="dashboard-fluid-chart">
		<Suspense fallback={<ChartFallback />}>
			<SurfaceChart panel={panel} />
		</Suspense>
	</div>
);

export const DashboardLayout = () => {
	const layout = useDashboardLayout();
	const chartSymbols = useOpenChartSymbols(layout.anchor_symbol);
	const gaugePanel = panelsByType(layout, "gauge_grid")[0];
	const gaugeStripPanel = panelsByType(layout, "gauge_strip")[0];
	const mergedGauge = mergedGaugePanel(layout);
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
									label: "Confidence",
									content: (
										<div className="h-full min-h-0 flex-1">
											<SignalHeatmap panel={mergedGauge} />
										</div>
									),
								},
							]}
						/>
					) : null}

					{/* Top-right: Gauges */}
					{gaugePanel !== undefined ? (
						<Gauges panel={gaugePanel} variant="grid" />
					) : null}
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
									<div className="dashboard-trade-chart-row h-full min-h-0 flex-1 overflow-hidden p-1">
										<div className="min-h-0 min-w-0 flex-1 overflow-hidden">
											<Suspense fallback={<ChartFallback />}>
												<TradeChartGrid symbols={chartSymbols} />
											</Suspense>
										</div>
										{gaugeStripPanel !== undefined ? (
											<Gauges panel={gaugeStripPanel} variant="strip" />
										) : null}
									</div>
								),
							},
							{
								key: "surprise",
								label: "Surprise",
								content: (
									<div className="h-full min-h-0 flex-1 p-1">
										<SignalSurpriseHeatmap panel={mergedGauge} />
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
