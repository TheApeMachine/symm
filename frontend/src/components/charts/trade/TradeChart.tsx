import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { initTradeChart } from "#/components/charts/trade/init-trade-chart";

export const TradeChart = memo(function TradeChart() {
	return (
		<SciChartReact
			style={{ flex: 1, width: "100%", height: "100%" }}
			initChart={initTradeChart}
			onInit={(result) => {
				appStore.actions.updateCandleUpdater(result.addData);
				return () => appStore.actions.updateCandleUpdater(null);
			}}
		/>
	);
});
