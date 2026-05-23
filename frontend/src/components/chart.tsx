import { TradeChart } from "#/components/symm/TradeChart";
import { EnginePulseChart } from "#/components/symm/EnginePulseChart";
import { useMarketWatchSymbol } from "#/lib/symm/use-symm-ui";

interface Props {
	connected: boolean;
	positionSymbols: string[];
}

export const ChartSection = ({ connected, positionSymbols }: Props) => {
	const watchSymbol = useMarketWatchSymbol();
	const tradeSymbols =
		positionSymbols.length > 0
			? positionSymbols
			: connected
				? [watchSymbol]
				: [];

	return (
		<section className="flex min-w-0 flex-7 flex-col overflow-hidden p-2">
			<div className="mt-2 grid min-h-[320px] flex-1 grid-rows-[140px_minmax(240px,1fr)] gap-2 overflow-hidden">
				<EnginePulseChart className="min-h-0" />
				{tradeSymbols.length > 0 ? (
					<div
						className={`grid min-h-0 gap-2 ${
							tradeSymbols.length === 1
								? "grid-cols-1"
								: "grid-cols-1 lg:grid-cols-2"
						}`}
					>
						{tradeSymbols.map((symbol) => (
							<TradeChart key={symbol} symbol={symbol} />
						))}
					</div>
				) : (
					<div className="flex min-h-[120px] items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) px-6 text-center text-sm text-(--dash-muted)">
						Start engine with make run
					</div>
				)}
			</div>
		</section>
	);
};
