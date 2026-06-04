import { ArrowDownRightIcon, ArrowUpRightIcon } from "lucide-react";
import { type ActionEvent, useWsStatus } from "#/providers/ws-status";

const ACTION_LABELS: Record<string, string> = {
	limit: "Limit",
	market: "Market",
	iceberg: "Iceberg",
	stop_loss: "Stop Loss",
	stop_loss_limit: "Stop Loss Limit",
	take_profit: "Take Profit",
	take_profit_limit: "Take Profit Limit",
	trailing_stop: "Trailing Stop",
	trailing_stop_limit: "Trailing Stop Limit",
	settle_position: "Settle",
};

const EXIT_TYPES = new Set([
	"settle_position",
	"stop_loss",
	"stop_loss_limit",
	"take_profit",
	"take_profit_limit",
	"trailing_stop",
	"trailing_stop_limit",
]);

const relativeTime = (ts: number) => {
	const seconds = Math.max(0, Math.round((Date.now() - ts) / 1000));

	if (seconds < 60) {
		return `${seconds}s ago`;
	}

	return `${Math.round(seconds / 60)}m ago`;
};

const ActionCard = ({ action }: { action: ActionEvent }) => {
	const isExit = EXIT_TYPES.has(action.type);
	const Icon = isExit ? ArrowDownRightIcon : ArrowUpRightIcon;
	const tone = isExit ? "text-amber-400" : "text-emerald-400";

	return (
		<div className="flex items-center gap-2 rounded-lg border border-border bg-card px-2.5 py-2">
			<Icon className={`size-4 shrink-0 ${tone}`} aria-hidden="true" />
			<div className="flex min-w-0 flex-1 flex-col">
				<div className="flex items-center justify-between gap-2">
					<span className="font-medium text-xs">
						{ACTION_LABELS[action.type] ?? action.type}
					</span>
					<span className="truncate font-mono text-[11px] text-muted-foreground">
						{action.symbol}
					</span>
				</div>
				<span className="text-[10px] uppercase tracking-wide text-muted-foreground">
					{isExit ? "exit" : "entry"} · {relativeTime(action.ts)}
				</span>
			</div>
		</div>
	);
};

export const Navigation = () => {
	const { actions } = useWsStatus();

	return (
		<div className="flex flex-col gap-1.5 p-2">
			<p className="px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
				Decisions
			</p>
			{actions.length === 0 ? (
				<p className="px-1 text-xs text-muted-foreground">No decisions yet</p>
			) : (
				actions.map((action) => (
					<ActionCard key={action.ts} action={action} />
				))
			)}
		</div>
	);
};
