import { memo } from "react";
import {
	useSymmConnected,
	useSymmStatus,
	useSymmPositionSymbols,
} from "#/lib/symm/use-symm-ui";
import ThemeToggle from "#/components/ThemeToggle";
import { Metric } from "./metric";
import { formatPnl, pnlTone } from "#/lib/utils";

export const DashboardHeader = memo(function DashboardHeader() {
	const connected = useSymmConnected();
	const status = useSymmStatus();
	const positionSymbols = useSymmPositionSymbols();

	return (
		<header className="flex h-11 shrink-0 items-center gap-3 border-b border-(--dash-border) px-3 sm:px-4">
			<div className="flex items-center gap-2">
				<span className="h-2 w-2 rounded-full bg-(--dash-accent)" />
				<span className="text-sm font-semibold tracking-[0.18em]">SYMM</span>
			</div>

			<span
				className={`rounded px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${
					connected
						? "bg-(--dash-live-bg) text-(--dash-live-text)"
						: "bg-(--dash-off-bg) text-(--dash-off-text)"
				}`}
			>
				{connected ? "live" : "offline"}
			</span>

			<span className="text-xs text-(--dash-muted)">
				{positionSymbols.length} open chart
				{positionSymbols.length === 1 ? "" : "s"}
			</span>

			<div className="ml-auto flex items-center gap-4 sm:gap-6">
				<Metric
					label="Equity"
					value={
						status ? `€${status.equity_eur.toFixed(2)}` : connected ? "—" : "…"
					}
				/>
				<Metric
					label="Closed PnL"
					value={
						status ? formatPnl(status.closed_pnl_eur) : connected ? "—" : "…"
					}
					tone={pnlTone(status?.closed_pnl_eur)}
				/>
				<Metric
					label="Open"
					value={status ? String(status.open_count) : connected ? "—" : "…"}
				/>
				<Metric
					label="Win rate"
					value={
						status
							? `${(status.win_rate * 100).toFixed(1)}%`
							: connected
								? "—"
								: "…"
					}
				/>
				<ThemeToggle />
			</div>
		</header>
	);
});
