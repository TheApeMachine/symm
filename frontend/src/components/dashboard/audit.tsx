import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "string" && value !== "" && Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

const posObj = new Position();
const holdingObj = new Holding();

export const AuditTrail = () => {
	const last = useSelector(positionStore, (state) =>
		state.findLast((f) => f.rowsLength() > 0),
	);

	const records: Array<{
		symbol: string;
		status: string;
		pnl: string;
		returnPct: string;
	}> = [];

	if (last) {
		for (let i = 0; i < last.rowsLength(); i++) {
			const pos = last.rows(i, posObj);
			if (!pos) continue;
			const h = pos.holding(holdingObj);
			if (!h) continue;
			const symbol = h.symbol() ?? "";
			if (!symbol) continue;

			records.push({
				symbol,
				status: h.status() ?? "—",
				pnl: fmt(h.pnl(), 4),
				returnPct: fmt(h.returnPct(), 4),
			});
		}
	}

	return (
		<div className="min-h-0 flex-1">
			<Typography.Span className="block border-(--line) border-b px-1 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-(--f3)">
				Audit trail
			</Typography.Span>
			<List className="min-h-0 border-(--line) border-b">
				{records.map((rec) => (
					<List.Item key={rec.symbol} className="justify-between" data-pos={rec.symbol}>
						<Typography.Span>{rec.status}</Typography.Span>
						<Typography.Span>{rec.symbol}</Typography.Span>
						<Typography.Span>
							pnl {rec.pnl} · return {rec.returnPct}%
						</Typography.Span>
					</List.Item>
				))}
			</List>
		</div>
	);
};


