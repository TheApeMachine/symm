import { terminalStore } from "#/collections/terminal";
import type { Position } from "#/collections/types";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { publishPositionExit } from "#/providers/websocket";
import { Flex } from "@/components/ui/flex";
import { positionsStore, useSubscribe } from "#/providers/ws-stores";
import { PositionStopGeometry } from "./position-stop-geometry";

const f = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : String(value ?? "—");

const openPositions = (): Position[] =>
	Object.values(positionsStore.state.positions)
		.map((buffer) => buffer.latest())
		.filter((row): row is Position => row !== undefined && row.status === "open");

export const Positions = () => {
	const root = useSubscribe(positionsStore, (state) => {
		const rows = Object.values(state.positions)
			.map((buffer) => buffer.latest())
			.filter((row): row is Position => row !== undefined && row.status === "open");

		for (const position of rows) {
			const card = root.current?.querySelector<HTMLElement>(
				`[data-pos="${position.holding.symbol}"]`,
			);

			if (card === null || card === undefined) {
				continue;
			}

			const set = (q: string, value: string) => {
				const el = card.querySelector<HTMLElement>(`[data-f="${q}"]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("symbol", position.holding.symbol);
			set("status", position.holding.status ?? "—");
			set("stoploss", position.holding.stoploss?.status ?? "—");
			set("pnl", `${f(position.holding.pnl, 4)} USD`);
			set("entry", f(position.holding.entry_price, 6));
			set("mark", f(position.holding.mark, 6));
			set("return", `${f(position.holding.return_pct, 2)}%`);
		}
	});

	return (
		<List ref={root} className="min-h-0 flex-1 p-1.5">
			{openPositions().map((position) => (
				<button
					type="button"
					data-pos={position.holding.symbol}
					data-position-card
					key={position.holding.symbol}
					onClick={() => terminalStore.actions.openThesis(position.holding.symbol)}
					title="Inspect this lot"
					className="mb-1.25 block w-full cursor-pointer rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 text-left font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
				>
					<Flex.Column className="gap-0">
						<Flex.Row className="items-center justify-between gap-2">
							<Flex.Row className="min-w-0 items-center gap-1.5">
								<Typography.Span data-f="symbol" className="font-semibold text-[11.5px] text-(--f1)" />
								<Typography.Span data-f="status" className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide" />
								<Typography.Span data-f="stoploss" className="text-[8px] uppercase text-(--f4)" />
							</Flex.Row>
							<Flex.Row className="items-center gap-1.5">
								<Typography.Span data-f="pnl" className="text-right font-semibold text-[11.5px] text-(--pnl)" />
								<button
									type="button"
									onClick={(event) => {
										event.preventDefault();
										event.stopPropagation();
										publishPositionExit(position.holding.symbol);
									}}
									title="Exit this position immediately"
									className="rounded-xs border border-(--down) px-1.5 py-px text-[8px] font-semibold text-(--down) uppercase tracking-wide hover:bg-[color-mix(in_srgb,var(--down)_12%,transparent)] disabled:cursor-wait disabled:opacity-60"
								>
									EXIT
								</button>
							</Flex.Row>
						</Flex.Row>

						<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
							<Typography.Span>
								entry <span data-f="entry" /> / mark <span data-f="mark" />
							</Typography.Span>
							<Typography.Span data-f="return" className="text-(--pnl)" />
						</Flex.Row>

						<PositionStopGeometry symbol={position.holding.symbol} />
					</Flex.Column>
				</button>
			))}
		</List>
	);
};