import { useWsStatus } from "#/providers/ws-status";

const ACTION_LABELS: Record<string, string> = {
	limit:               "Limit",
	market:              "Market",
	iceberg:             "Iceberg",
	stop_loss:           "Stop Loss",
	stop_loss_limit:     "Stop Loss Limit",
	take_profit:         "Take Profit",
	take_profit_limit:   "Take Profit Limit",
	trailing_stop:       "Trailing Stop",
	trailing_stop_limit: "Trailing Stop Limit",
	settle_position:     "Settle",
};

export const Navigation = () => {
	const { actions } = useWsStatus();

	return (
		<div className="flex flex-col gap-1 p-2 text-xs">
			<p className="px-1 py-0.5 font-semibold text-muted-foreground uppercase tracking-wide text-[10px]">
				Actions
			</p>
			{actions.length === 0 ? (
				<p className="px-1 text-muted-foreground">No actions yet</p>
			) : (
				actions.map((a) => (
					<div
						key={a.ts}
						className="flex items-center justify-between rounded border border-border px-2 py-1 bg-card"
					>
						<span className="font-medium">{ACTION_LABELS[a.type] ?? a.type}</span>
						<span className="text-muted-foreground">{a.symbol}</span>
					</div>
				))
			)}
		</div>
	);
};
