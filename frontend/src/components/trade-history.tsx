import { ArrowDownRightIcon, ArrowUpRightIcon, MinusIcon } from "lucide-react";
import type {
	TradeHistoryOutcome,
	TradeHistoryRow,
} from "#/components/panels/data/trade-history-data-provider";
import {
	useSymmConnected,
	useSymmTradeHistoryRows,
} from "#/lib/symm/use-dashboard-data";

const formatSignedEur = (value: number) => {
	const prefix = value >= 0 ? "+" : "−";

	return `${prefix}€${Math.abs(value).toFixed(4)}`;
};

const formatPrice = (value: number) => {
	if (Math.abs(value) < 1) {
		return value.toFixed(4);
	}

	return value.toFixed(2);
};

const formatTime = (value: string) => {
	const parsed = Date.parse(value);

	if (!Number.isFinite(parsed)) {
		return value;
	}

	return new Intl.DateTimeFormat(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
		hour12: false,
	}).format(parsed);
};

const outcomeMeta: Record<
	TradeHistoryOutcome,
	{ label: string; tone: string; Icon: typeof ArrowUpRightIcon }
> = {
	profit: {
		label: "profit",
		tone: "text-emerald-400",
		Icon: ArrowUpRightIcon,
	},
	loss: {
		label: "loss",
		tone: "text-rose-400",
		Icon: ArrowDownRightIcon,
	},
	flat: {
		label: "flat",
		tone: "text-muted-foreground",
		Icon: MinusIcon,
	},
};

const HistoryRow = ({ row }: { row: TradeHistoryRow }) => {
	const outcome = outcomeMeta[row.outcome];

	return (
		<li className="rounded-lg border border-border bg-card px-2.5 py-2">
			<div className="flex items-start justify-between gap-2">
				<div className="min-w-0 flex-1">
					<div className="flex items-center gap-1.5">
						<outcome.Icon
							className={`size-3.5 shrink-0 ${outcome.tone}`}
							aria-hidden="true"
						/>
						<span className="truncate font-medium text-xs">{row.symbol}</span>
					</div>
					{row.reason ? (
						<p className="mt-0.5 truncate text-[11px] text-muted-foreground">
							{row.reason}
						</p>
					) : null}
				</div>
				<div className="shrink-0 text-right">
					<div className={`tabular-nums text-xs font-medium ${outcome.tone}`}>
						{formatSignedEur(row.realizedEur)}
					</div>
					{row.realizedPct !== undefined ? (
						<div className={`tabular-nums text-[10px] ${outcome.tone}`}>
							{row.realizedPct >= 0 ? "+" : "−"}
							{Math.abs(row.realizedPct).toFixed(2)}%
						</div>
					) : null}
				</div>
			</div>
			<div className="mt-1 flex items-center justify-between gap-2 text-[10px] uppercase tracking-wide text-muted-foreground">
				<span>{outcome.label}</span>
				<span className="tabular-nums normal-case">
					{formatTime(row.closedAt)}
				</span>
			</div>
			{row.entryPrice !== undefined || row.exitPrice !== undefined ? (
				<div className="mt-1 text-[10px] tabular-nums text-muted-foreground">
					{row.qty !== undefined ? `${row.qty.toFixed(6)} · ` : ""}
					{row.entryPrice !== undefined
						? `entry ${formatPrice(row.entryPrice)}`
						: ""}
					{row.exitPrice !== undefined
						? ` · exit ${formatPrice(row.exitPrice)}`
						: ""}
				</div>
			) : null}
		</li>
	);
};

export const TradeHistoryPanel = () => {
	const connected = useSymmConnected();
	const rows = useSymmTradeHistoryRows();

	return (
		<div className="flex min-h-0 flex-col gap-1.5">
			<p className="px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
				Trade history
			</p>
			{rows.length === 0 ? (
				<p className="px-1 text-xs text-muted-foreground">
					{connected ? "No closed trades yet" : "Connect to load trade history"}
				</p>
			) : (
				<ul className="space-y-1.5">
					{rows.map((row) => (
						<HistoryRow key={row.key} row={row} />
					))}
				</ul>
			)}
		</div>
	);
};
