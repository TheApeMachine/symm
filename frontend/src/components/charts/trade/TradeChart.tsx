import { memo, useCallback } from "react";
import { SciChartGroup, SciChartReact } from "scichart-react";

import {
	initTradeChart,
	type TTradeChartInitResult,
} from "#/components/charts/trade/init-trade-chart";
import { registerTradeChart } from "#/components/charts/trade/trade-chart-wire";

type TradeChartProps = {
	symbol: string;
	className?: string;
};

export const TradeChart = memo(function TradeChart({
	symbol,
	className = "",
}: TradeChartProps) {
	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => {
			if (typeof rootElement === "string") {
				throw new Error("initTradeChart requires an HTMLDivElement root");
			}

			return initTradeChart(rootElement, symbol);
		},
		[symbol],
	);

	const onInit = useCallback(
		(result: TTradeChartInitResult) =>
			registerTradeChart(symbol, result.appendBar),
		[symbol],
	);

	return (
		<SciChartReact
			initChart={initChart}
			onInit={onInit}
			className={`h-full w-full ${className}`}
			innerContainerProps={{ className: "h-full w-full flex-1" }}
			style={{ width: "100%", height: "100%" }}
		/>
	);
});

type TradeChartGridProps = {
	symbols: string[];
};

export const TradeChartGrid = memo(function TradeChartGrid({
	symbols,
}: TradeChartGridProps) {
	const gridClass =
		symbols.length === 1 ? "grid-cols-1" : "grid-cols-1 lg:grid-cols-2";

	return (
		<div className={`grid h-full w-full flex-1 gap-1 ${gridClass}`}>
			<SciChartGroup>
				{symbols.map((symbol) => (
					<TradeChart
						key={symbol}
						symbol={symbol}
						className="h-full w-full flex-1"
					/>
				))}
			</SciChartGroup>
		</div>
	);
});
