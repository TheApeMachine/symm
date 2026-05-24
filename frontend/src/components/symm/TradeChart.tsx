import { memo, useCallback, useRef } from "react";
import { SciChartGroup, SciChartReact } from "scichart-react";

import {
	initTradeChart,
	type TradeChartInitResult,
} from "#/lib/symm/chart-controller";
import {
	registerTradeChartBridge,
	subscribeTradeChart,
	unregisterTradeChartBridge,
	unsubscribeTradeChart,
} from "#/lib/symm/feed";
import { useSymmStatus } from "#/lib/symm/use-symm-ui";
import "#/lib/symm/scichart-setup";

type TradeChartProps = {
	symbol: string;
};

/** One SciChart surface per symbol — data flows store → bridge → append. */
export const TradeChart = memo(function TradeChart({
	symbol,
}: TradeChartProps) {
	const status = useSymmStatus();
	const regime =
		status?.positions?.find((position) => position.symbol === symbol)?.regime ??
		"";

	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => initTradeChart(rootElement),
		[],
	);

	const onInit = useCallback(
		(result: TradeChartInitResult) => {
			registerTradeChartBridge(symbol, result);
			subscribeTradeChart(symbol);
		},
		[symbol],
	);

	const onDelete = useCallback(
		(result?: TradeChartInitResult) => {
			result.dispose();
			unregisterTradeChartBridge(symbol);
			unsubscribeTradeChart(symbol);
		},
		[symbol],
	);

	return (
		<div className="flex min-h-[200px] flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel)">
			<div className="flex items-center justify-between border-b border-(--dash-border) px-2 py-1">
				<span className="text-xs font-semibold">{symbol}</span>
				<span className="text-[10px] text-(--dash-muted)">{regime}</span>
			</div>
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				onDelete={onDelete}
				className="min-h-[180px] flex-1"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
		</div>
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
		<div className={`grid min-h-0 gap-2 ${gridClass}`}>
			<SciChartGroup>
				{symbols.map((symbol) => (
					<TradeChart key={symbol} symbol={symbol} />
				))}
			</SciChartGroup>
		</div>
	);
});
