import {
	useSymmConnected,
	useSymmTradePanelRows,
} from "#/lib/symm/use-symm-ui";
import { SidebarSection } from "./sidebar";
import { EmptyHint } from "./hint";
import { formatEur, formatPnl, pnlTone } from "#/lib/utils";

export const TradesPanel = () => {
	const connected = useSymmConnected();
	const rows = useSymmTradePanelRows();

	return (
		<SidebarSection
			title="Trades"
			fill
			className="min-w-0 border-l border-(--dash-border)"
		>
			{rows.length === 0 ? (
				<EmptyHint connected={connected} />
			) : (
				<ul className="space-y-1 px-2 pb-2">
					{rows.slice(0, 20).map((row) => (
						<li
							key={row.key}
							className="rounded border border-(--dash-border) px-2 py-1.5 text-xs"
						>
							<div className="flex items-center justify-between gap-2">
								<span className="font-medium">
									{row.kind === "exit"
										? "EXIT"
										: row.kind === "open"
											? "OPEN"
											: "ENTER"}{" "}
									{row.symbol}
								</span>
								{row.kind === "open" && row.open_pnl_eur !== undefined ? (
									<span className={`tabular-nums ${pnlTone(row.open_pnl_eur)}`}>
										{formatPnl(row.open_pnl_eur)}
									</span>
								) : null}
								{row.kind === "exit" && row.pnl_eur !== undefined ? (
									<span className={`tabular-nums ${pnlTone(row.pnl_eur)}`}>
										{formatPnl(row.pnl_eur)}
									</span>
								) : null}
							</div>
							<div className="mt-0.5 flex items-center justify-between gap-2 text-(--dash-muted)">
								<span>
									{row.regime} · {row.reason}
								</span>
								{row.kind === "open" && row.notional_eur !== undefined ? (
									<span className="tabular-nums">
										{formatEur(row.notional_eur)}
									</span>
								) : null}
							</div>
						</li>
					))}
				</ul>
			)}
		</SidebarSection>
	);
};
