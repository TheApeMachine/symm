import { memo, useCallback } from "react";
import {
	SciChartGroup,
	SciChartReact,
	type TResolvedReturnType,
} from "scichart-react";

import { drawTradeChart } from "#/components/symm/draw-trade-chart";
import { onChart } from "#/lib/symm/feed";
import type { SymmEvent } from "#/lib/symm/events";
import "#/lib/symm/scichart-setup";

type TradeChartProps = {
	symbol: string;
};

/** One SciChart surface per open position — ticks arrive via feed, not React props. */
export const TradeChart = memo(function TradeChart({
	symbol,
}: TradeChartProps) {
	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => {
			if (typeof rootElement === "string") {
				throw new Error("drawTradeChart requires an HTMLDivElement root");
			}

			return drawTradeChart(rootElement, symbol);
		},
		[symbol],
	);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawTradeChart>) =>
			onChart(symbol, (ev: SymmEvent) => {
				result.controls.handleEvent(ev);
			}),
		[symbol],
	);

	return (
		<SciChartReact
			initChart={initChart}
			onInit={onInit}
			className="h-full min-h-[180px] w-full"
			innerContainerProps={{ className: "h-full w-full" }}
		/>
	);
});

type TradeChartGridProps = {
	symbols: string[];
};

/** Grid of linked trade charts — SciChartGroup shares modifier context across surfaces. */
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
