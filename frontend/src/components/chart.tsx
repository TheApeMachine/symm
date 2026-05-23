import { TradeChartGrid } from "#/components/symm/TradeChart";

interface Props {
	connected: boolean;
	positionSymbols: string[];
}

export const ChartSection = ({ connected, positionSymbols }: Props) => {
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
						<TradeChartGrid symbols={positionSymbols} />
					</div>
				) : (
					<div className="flex h-full min-h-[120px] items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) px-6 text-center text-sm text-(--dash-muted)">
						{connected
							? "No open positions — trade charts appear on entry"
							: "Start engine with make run"}
					</div>
				)}
			</div>
		</section>
	);
};
