import { Suspense, lazy, useMemo } from "react";

import { AuditPanel } from "#/components/audit";
import { TradesPanel } from "#/components/trades";
import { Gauges } from "#/components/gauges";
import { panelsByType, type LayoutPanel } from "#/lib/symm/layout-schema";
import { useDashboardLayout } from "#/lib/symm/use-dashboard-layout";
import { useSymmWallet } from "#/lib/symm/use-dashboard-data";

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

	return (
		<div className="dashboard-workspace">
			<section className="dashboard-primary">
				<div className="dashboard-top-row">
					{panelsByType(layout, "prediction_chart").length > 0 ? (
						<Suspense fallback={<ChartFallback />}>
							<PredictionChart />
						</Suspense>
					) : null}
					{gaugePanel !== undefined ? <Gauges panel={gaugePanel} /> : null}
				</div>
				{panelsByType(layout, "trade_grid").length > 0 ? (
					<div className="dashboard-trade-panel">
						<Suspense fallback={<ChartFallback />}>
							<TradeChartGrid symbols={chartSymbols} />
						</Suspense>
					</div>
				) : null}
			</section>
			<section className="dashboard-secondary">
				<div className="dashboard-trades-strip">
					{panelsByType(layout, "trades_panel").length > 0 ? (
						<TradesPanel />
					) : null}
					{panelsByType(layout, "audit_panel").length > 0 ? (
						<AuditPanel />
					) : null}
				</div>
				{surfacePanel !== undefined ? (
					<SurfacePanel panel={surfacePanel} />
				) : null}
			</section>
		</div>
	);
};
