import {
	useSymmConnected,
	useSymmTradePanelRows,
} from "#/lib/symm/use-dashboard-data";
import { SidebarSection } from "./sidebar-section";
import { EmptyHint } from "./hint";
import type { TradePanelRow } from "./symm/trades-data-provider";

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

const pnlClass = (value: number | undefined) => {
	if (value === undefined || value === 0) {
		return "text-(--dash-muted)";
	}

	return value > 0 ? "text-emerald-400" : "text-rose-400";
};

const OpenRow = ({ row }: { row: TradePanelRow }) => (
	<li className="rounded border border-(--dash-border) px-2 py-1.5 text-xs">
		<div className="flex items-center justify-between gap-2">
			<span className="font-medium">OPEN {row.symbol}</span>
			{row.unrealizedEur !== undefined ? (
				<span
					className={`tabular-nums font-medium ${pnlClass(row.unrealizedEur)}`}
				>
					<span className="mr-1 text-(--dash-muted)">P/L</span>
					{formatSignedEur(row.unrealizedEur)}
				</span>
			) : row.qty !== undefined ? (
				<span className="tabular-nums text-(--dash-muted)">
					{row.qty.toFixed(6)}
				</span>
			) : null}
		</div>
		<div className="mt-0.5 flex items-center justify-between gap-2 text-(--dash-muted)">
			<span>
				open
				{row.entryPrice !== undefined
					? ` · entry ${formatPrice(row.entryPrice)}`
					: ""}
				{row.markPrice !== undefined
					? ` · mark ${formatPrice(row.markPrice)}`
					: ""}
			</span>
			{row.unrealizedPct !== undefined ? (
				<span className={`tabular-nums ${pnlClass(row.unrealizedEur)}`}>
					{row.unrealizedPct >= 0 ? "+" : "−"}
					{Math.abs(row.unrealizedPct).toFixed(2)}%
				</span>
			) : (
				<span className="tabular-nums">inventory</span>
			)}
		</div>
	</li>
);

const FillRow = ({ row }: { row: TradePanelRow }) => {
	const label = row.kind === "enter" ? "BUY" : "SELL";
	const labelClass =
		row.kind === "enter" ? "text-emerald-400" : "text-rose-400";

	return (
		<li className="rounded border border-(--dash-border) px-2 py-1.5 text-xs opacity-70">
			<div className="flex items-center justify-between gap-2">
				<span className="font-medium">
					<span className={labelClass}>{label}</span> {row.symbol}
				</span>
				{row.notionalEur !== undefined ? (
					<span className="tabular-nums text-(--dash-muted)">
						€{row.notionalEur.toFixed(2)}
					</span>
				) : null}
			</div>
			<div className="mt-0.5 flex items-center justify-between gap-2 text-(--dash-muted)">
				<span>fill</span>
				{row.price !== undefined && row.qty !== undefined ? (
					<span className="tabular-nums">
						{row.qty.toFixed(6)} @ {formatPrice(row.price)}
					</span>
				) : null}
			</div>
		</li>
	);
};

export const TradesPanel = () => {
	const connected = useSymmConnected();
	const rows = useSymmTradePanelRows();

	return (
		<SidebarSection title="Trades" fill className="min-h-0 min-w-0">
			{rows.length === 0 ? (
				<EmptyHint
					connected={connected}
					message={connected ? "No open positions" : undefined}
				/>
			) : (
				<ul className="space-y-1 px-2 pb-2">
					{rows.slice(0, 24).map((row) =>
						row.kind === "open" ? (
							<OpenRow key={row.key} row={row} />
						) : (
							<FillRow key={row.key} row={row} />
						),
					)}
				</ul>
			)}
		</SidebarSection>
	);
};
