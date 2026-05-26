import { memo, useCallback } from "react";
import { SciChartGroup, SciChartReact } from "scichart-react";

import {
	initTradeChart,
	type TTradeChartInitResult,
} from "#/components/symm/init-trade-chart";
import { registerTradeChart } from "#/components/symm/ws";

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
			className={`min-h-0 w-full flex-1 ${className}`}
			innerContainerProps={{ className: "h-full w-full" }}
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
		<div className={`grid min-h-0 flex-1 gap-0 ${gridClass}`}>
			<SciChartGroup>
				{symbols.map((symbol) => (
					<TradeChart key={symbol} symbol={symbol} />
				))}
			</SciChartGroup>
		</div>
	);
});
