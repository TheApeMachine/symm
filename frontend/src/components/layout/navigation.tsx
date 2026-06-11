import { Link } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import {
	ActivityIcon,
	ArrowDownRightIcon,
	ArrowUpRightIcon,
	BanIcon,
	CheckCircle2Icon,
	HomeIcon,
	LoaderIcon,
	NetworkIcon,
} from "lucide-react";
import { statusStore } from "#/collections/status";

const PAGES: { to: string; label: string; Icon: typeof HomeIcon }[] = [
	{ to: "/", label: "Dashboard", Icon: HomeIcon },
	{ to: "/diagnostics", label: "Signal Insight", Icon: ActivityIcon },
	{ to: "/decisions", label: "Decision Tree", Icon: NetworkIcon },
];

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

const VERDICT_META: Record<
	string,
	{ Icon: typeof BanIcon; tone: string; label: string }
> = {
	filled: { Icon: CheckCircle2Icon, tone: "text-emerald-400", label: "filled" },
	submitted: { Icon: LoaderIcon, tone: "text-sky-400", label: "submitted" },
	rejected: { Icon: BanIcon, tone: "text-rose-400", label: "blocked" },
};

const ActionCard = ({
	action,
}: {
	action: {
		type: string;
		symbol: string;
		reason?: string;
		verdict: string;
		ts: number;
	};
}) => {
	const isExit = EXIT_TYPES.has(action.type);
	const DirectionIcon = isExit ? ArrowDownRightIcon : ArrowUpRightIcon;
	const verdict = VERDICT_META[action.verdict] ?? VERDICT_META.rejected;

	return (
		<div className="flex items-start gap-2 rounded-lg border border-border bg-card px-2.5 py-2">
			<verdict.Icon
				className={`mt-0.5 size-4 shrink-0 ${verdict.tone}`}
				aria-hidden="true"
			/>
			<div className="flex min-w-0 flex-1 flex-col gap-0.5">
				<div className="flex items-center justify-between gap-2">
					<span className="flex items-center gap-1 font-medium text-xs">
						<DirectionIcon
							className="size-3 shrink-0 text-muted-foreground"
							aria-hidden="true"
						/>
						{ACTION_LABELS[action.type] ?? action.type}
					</span>
					<span className="truncate font-mono text-[11px] text-muted-foreground">
						{action.symbol}
					</span>
				</div>
				{action.reason ? (
					<span className={`truncate text-[11px] ${verdict.tone}`}>
						{action.reason}
					</span>
				) : null}
				<span className="text-[10px] uppercase tracking-wide text-muted-foreground">
					{isExit ? "exit" : "entry"} · {verdict.label} ·{" "}
					{relativeTime(action.ts)}
				</span>
			</div>
		</div>
	);
};

export const Navigation = ({ onNavigate }: { onNavigate?: () => void }) => {
	const { actions } = useSelector(statusStore, (state) => state);

	return (
		<div className="flex flex-col gap-1.5 p-2">
			<p className="px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
				Pages
			</p>
			{PAGES.map((page) => (
				<Link
					key={page.to}
					to={page.to}
					onClick={onNavigate}
					className="flex items-center gap-2 rounded-lg border border-border bg-card px-2.5 py-2 text-sm hover:bg-muted [&.active]:border-sky-500/50 [&.active]:bg-sky-500/10"
				>
					<page.Icon className="size-4 shrink-0 text-muted-foreground" />
					{page.label}
				</Link>
			))}
			<p className="mt-2 px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
				Decisions
			</p>
			{actions.length === 0 ? (
				<p className="px-1 text-xs text-muted-foreground">No decisions yet</p>
			) : (
				actions.map((action) => (
					<ActionCard key={action.symbol} action={action} />
				))
			)}
		</div>
	);
};
