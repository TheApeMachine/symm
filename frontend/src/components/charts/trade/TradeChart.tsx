import { useSelector } from "@tanstack/react-store";
import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { statusStore } from "#/collections/status";
import { initTradeChart } from "#/components/charts/trade/init-trade-chart";
import { cn } from "#/lib/utils";

type TradeChartProps = {
	symbol: string;
};

export const TradeChart = memo(({ symbol }: TradeChartProps) => {
	return (
		<div className="relative min-h-0 h-full w-full overflow-hidden">
			<div className="pointer-events-none absolute left-3 top-2 z-10 rounded-sm bg-background/80 px-2 py-1 font-mono text-xs font-semibold">
				{symbol}
			</div>
			<SciChartReact
				style={{ flex: 1, width: "100%", height: "100%" }}
				initChart={(rootElement) => initTradeChart(rootElement, symbol)}
				onInit={(result) => {
					appStore.actions.updateCandleUpdater(symbol, result.addData);
					return () => appStore.actions.updateCandleUpdater(symbol, null);
				}}
			/>
		</div>
	);
});

export const PositionTradeCharts = () => {
	const positions = useSelector(
		statusStore,
		(state) => state.positionViews,
	);
	const symbols = positions.map((position) => position.symbol);

	if (symbols.length === 0) {
		return (
			<div className="grid h-full w-full place-items-center text-sm text-muted-foreground">
				No open positions
			</div>
		);
	}

	return (
		<div
			className={cn(
				"grid h-full w-full min-h-0 gap-px bg-border",
				symbols.length === 1
					? "grid-cols-1"
					: "grid-cols-1 xl:grid-cols-2",
			)}
			style={{ gridAutoRows: "minmax(0, 1fr)" }}
		>
			{symbols.map((symbol) => (
				<div
					key={symbol}
					className="min-h-0 min-w-0 overflow-hidden bg-background"
				>
					<TradeChart symbol={symbol} />
				</div>
			))}
		</div>
	);
};
