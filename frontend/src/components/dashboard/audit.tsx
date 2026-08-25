import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { positionsStore, useSubscribe } from "#/providers/ws-stores";

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : String(value ?? "—");

export const AuditTrail = () => {
	const root = useSubscribe(positionsStore, (state) => {
		const rows = Object.values(state.positions).map((buffer) => buffer.latest());

		for (const position of rows) {
			if (position === undefined) {
				continue;
			}

			const cell = root.current?.querySelector<HTMLElement>(
				`[data-pos="${position.holding.symbol}"]`,
			);

			if (cell === null || cell === undefined) {
				continue;
			}

			const set = (q: string, value: string) => {
				const el = cell.querySelector<HTMLElement>(`[data-f="${q}"]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("status", position.holding.status ?? "—");
			set("symbol", position.holding.symbol);
			set("pnl", fmt(position.holding.pnl, 4));
			set("return", `${fmt(position.holding.return_pct, 4)}%`);
		}
	});

	return (
		<div ref={root} className="min-h-0 flex-1">
			<Typography.Span className="block border-(--line) border-b px-1 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-(--f3)">
				Audit trail
			</Typography.Span>
			<List className="min-h-0 border-(--line) border-b">
				{Object.values(positionsStore.state.positions)
					.map((buffer) => buffer.latest())
					.filter((row) => row !== undefined)
					.map((position) => (
						<List.Item key={position.holding.symbol} className="justify-between" data-pos={position.holding.symbol}>
							<Typography.Span data-f="status" />
							<Typography.Span data-f="symbol" />
							<Typography.Span>
								pnl <span data-f="pnl" /> · return{" "}
								<span data-f="return" />
								%
							</Typography.Span>
						</List.Item>
					))}
			</List>
		</div>
	);
};
