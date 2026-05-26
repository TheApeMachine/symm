import { memo } from "react";

import ThemeToggle from "#/components/ThemeToggle";
import { Metric } from "./metric";
import {
	useSymmConnected,
	useSymmEnginePulse,
	useSymmWallet,
} from "#/lib/symm/use-dashboard-data";
import { formatEur } from "#/lib/utils";

export const DashboardHeader = memo(function DashboardHeader() {
	const connected = useSymmConnected();
	const wallet = useSymmWallet();
	const pulse = useSymmEnginePulse();
	const cash = wallet.balance + wallet.reservedEur;

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
				{wallet.openCount} open position{wallet.openCount === 1 ? "" : "s"}
			</span>

			<div className="ml-auto flex items-center gap-4 sm:gap-6">
				<Metric label="Cash" value={connected ? formatEur(cash) : "…"} />
				<Metric
					label="Available"
					value={connected ? formatEur(wallet.balance) : "…"}
				/>
				<Metric
					label="Reserved"
					value={connected ? formatEur(wallet.reservedEur) : "…"}
				/>
				<Metric
					label="Tick"
					value={connected ? String(pulse?.seq ?? 0) : "…"}
				/>
				<ThemeToggle />
			</div>
		</header>
	);
});
