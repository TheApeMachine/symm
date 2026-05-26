import {
	useSymmConnected,
	useSymmTradePanelRows,
} from "#/lib/symm/use-dashboard-data";
import { SidebarSection } from "./sidebar-section";
import { EmptyHint } from "./hint";
import { formatEur } from "#/lib/utils";

export const TradesPanel = () => {
	const connected = useSymmConnected();
	const rows = useSymmTradePanelRows();

	return (
		<SidebarSection title="Trades" fill className="min-h-0 min-w-0">
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
								{row.kind === "open" && row.qty !== undefined ? (
									<span className="tabular-nums text-(--dash-muted)">
										{row.qty.toFixed(6)}
									</span>
								) : null}
								{row.kind !== "open" && row.notionalEur !== undefined ? (
									<span className="tabular-nums text-(--dash-text)">
										{formatEur(row.notionalEur)}
									</span>
								) : null}
							</div>
							<div className="mt-0.5 flex items-center justify-between gap-2 text-(--dash-muted)">
								<span>
									{row.side ?? row.kind}
									{row.price !== undefined ? ` @ ${row.price.toFixed(2)}` : ""}
								</span>
								{row.kind === "open" && row.qty !== undefined ? (
									<span className="tabular-nums">inventory</span>
								) : null}
							</div>
						</li>
					))}
				</ul>
			)}
		</SidebarSection>
	);
};
