import { TradeChart } from '#/components/symm/TradeChart'
import { EnginePulseChart } from '#/components/symm/EnginePulseChart'
import { useMarketWatchSymbol } from '#/lib/symm/use-symm-ui'

interface Props {
	connected: boolean
	positionSymbols: string[]
}

export const ChartSection = ({ connected, positionSymbols }: Props) => {
	const watchSymbol = useMarketWatchSymbol()

	return (
		<section className="flex min-w-0 flex-7 flex-col overflow-hidden p-2">
			<div className="mt-2 min-h-0 flex-1 overflow-auto">
				{positionSymbols.length > 0 ? (
					<div
						className={`grid h-full gap-2 ${
							positionSymbols.length === 1
								? "grid-cols-1"
								: positionSymbols.length === 2
									? "grid-cols-1 lg:grid-cols-2"
									: "grid-cols-1 lg:grid-cols-2 xl:grid-cols-2"
						}`}
					>
						{positionSymbols.map((symbol) => (
							<TradeChart key={symbol} symbol={symbol} />
						))}
					</div>
				) : (
					<div className="grid h-full min-h-[320px] grid-rows-[minmax(160px,1fr)_minmax(200px,1.2fr)] gap-2">
						<EnginePulseChart className="min-h-0" />
						{connected ? (
							<TradeChart symbol={watchSymbol} />
						) : (
							<div className="flex min-h-[120px] items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) px-6 text-center text-sm text-(--dash-muted)">
								Start engine with make run
							</div>
						)}
					</div>
				)}
			</div>
		</section>
	)
};
