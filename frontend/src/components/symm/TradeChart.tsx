import { memo, useCallback, useRef, type RefObject } from "react";
import { SciChartGroup, SciChartReact, type TResolvedReturnType } from "scichart-react";

import { drawTradeChart } from "#/components/symm/draw-trade-chart";
import { onChart } from "#/lib/symm/feed";
import type { StatusEvent, SymmEvent } from "#/lib/symm/events";
import "#/lib/symm/scichart-setup";

type TradeChartProps = {
	symbol: string;
};

function syncRegimeLabel(
	symbol: string,
	ev: SymmEvent,
	regimeRef: RefObject<HTMLSpanElement | null>,
) {
	if (ev.event !== "status") return;
	const pos = (ev as StatusEvent).positions?.find((p) => p.symbol === symbol);
	if (pos && regimeRef.current) {
		regimeRef.current.textContent = pos.regime;
	}
}

/** One SciChart surface per open position — ticks arrive via feed, not React props. */
export const TradeChart = memo(function TradeChart({
	symbol,
}: TradeChartProps) {
	const regimeRef = useRef<HTMLSpanElement>(null);

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
			onChart(symbol, (ev) => {
				result.controls.handleEvent(ev);
				syncRegimeLabel(symbol, ev, regimeRef);
			}),
		[symbol],
	);

	return (
		<div className="flex min-h-[200px] flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel)">
			<div className="flex items-center justify-between border-b border-(--dash-border) px-2 py-1">
				<span className="text-xs font-semibold">{symbol}</span>
				<span ref={regimeRef} className="text-[10px] text-(--dash-muted)" />
			</div>
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
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
