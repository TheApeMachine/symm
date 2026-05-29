import {
	useSymmAuditRows,
	useSymmConnected,
} from "#/lib/symm/use-dashboard-data";
import { EmptyHint } from "./hint";
import { SidebarSection } from "./sidebar-section";

const formatTime = (value: string) => {
	const parsed = Date.parse(value);

	if (!Number.isFinite(parsed)) {
		return value;
	}

	return new Intl.DateTimeFormat(undefined, {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		hour12: false,
	}).format(parsed);
};

export const AuditPanel = () => {
	const connected = useSymmConnected();
	const rows = useSymmAuditRows();

	return (
		<SidebarSection title="Audit" fill className="min-h-0 min-w-0">
			{rows.length === 0 ? (
				<EmptyHint
					connected={connected}
					message={connected ? "Waiting for audit events…" : undefined}
				/>
			) : (
				<ul className="space-y-1 px-2 pb-2">
					{rows.map((row) => (
						<li
							key={row.key}
							className="border-t border-(--dash-border) py-1.5 text-[11px]"
						>
							<div className="flex items-center justify-between gap-2">
								<span className="truncate font-medium">{row.event}</span>
								<span className="shrink-0 tabular-nums text-(--dash-muted)">
									#{row.seq} · {formatTime(row.ts)}
								</span>
							</div>
							<div className="mt-0.5 truncate text-(--dash-muted)">
								{[row.symbol, row.source, row.reason]
									.filter(Boolean)
									.join(" · ")}
							</div>
							{row.summary ? (
								<div className="mt-0.5 truncate tabular-nums text-(--dash-muted)">
									{row.summary}
								</div>
							) : null}
						</li>
					))}
				</ul>
			)}
		</SidebarSection>
	);
};
