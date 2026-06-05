import { Flex } from "#/components/ui/flex";
import { useWsStatus } from "#/providers/ws-status";

const eur = (value: number) =>
	`${value < 0 ? "-" : ""}€${Math.abs(value).toFixed(2)}`;

const signed = (value: number) =>
	`${value >= 0 ? "+" : "-"}€${Math.abs(value).toFixed(2)}`;

/*
PositionsPanel lists the open book with live unrealized P&L, marked against the
OHLC stream. Rendered as a dropdown from the wallet button.
*/
export const PositionsPanel = () => {
	const { positionViews } = useWsStatus();

	return (
		<Flex.Column gap={2}>
			<p className="px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
				Open positions
			</p>

			{positionViews.length === 0 ? (
				<p className="px-1 py-2 text-xs text-muted-foreground">
					No open positions
				</p>
			) : (
				positionViews.map((position) => {
					const positive = position.unrealized >= 0;

					return (
						<div
							key={position.symbol}
							className="flex flex-col gap-1 rounded-lg border border-border bg-background px-3 py-2"
						>
							<div className="flex items-center justify-between">
								<span className="font-semibold text-sm">{position.symbol}</span>
								<span
									className={`font-mono text-sm ${positive ? "text-emerald-400" : "text-red-400"}`}
								>
									{signed(position.unrealized)}
								</span>
							</div>
							<div className="flex items-center justify-between text-[11px] text-muted-foreground">
								<span>
									{position.qty.toFixed(4)} @ {eur(position.avgEntry)}
								</span>
								<span
									className={positive ? "text-emerald-400" : "text-red-400"}
								>
									{position.unrealizedPct >= 0 ? "+" : ""}
									{position.unrealizedPct.toFixed(2)}%
								</span>
							</div>
							<div className="flex items-center justify-between text-[11px] text-muted-foreground">
								<span>mark</span>
								<span className="font-mono">{eur(position.mark)}</span>
							</div>
						</div>
					);
				})
			)}
		</Flex.Column>
	);
};
