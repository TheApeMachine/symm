import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { positionStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";
import { sendPositionExit } from "#/providers/websocket";
import { PositionStopGeometry } from "./position-stop-geometry";

const f = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "string" && value !== "" && Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

const posObj = new Position();
const holdingObj = new Holding();
const stoplossObj = new Stoploss();

export const Positions = () => {
	const last = useSelector(positionStore, (state) =>
		state.findLast((f) => f.rowsLength() > 0),
	);
	const [pendingExits, setPendingExits] = useState<ReadonlySet<string>>(new Set());

	const positions: Array<{
		symbol: string;
		status: string;
		stoploss: string;
		pnl: string;
		entryPrice: string;
		mark: string;
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

			const stoploss = h.stoploss(stoplossObj);

			positions.push({
				symbol,
				status: h.status() ?? "—",
				stoploss: stoploss?.status() ?? "—",
				pnl: `${f(h.pnl(), 4)} USD`,
				entryPrice: f(h.entryPrice(), 6),
				mark: f(h.mark(), 6),
				returnPct: `${f(h.returnPct(), 2)}%`,
			});
		}
	}

	const requestExit = (symbol: string) => {
		if (pendingExits.has(symbol)) {
			return;
		}

		setPendingExits((current) => new Set(current).add(symbol));
		sendPositionExit(symbol);
	};

	return (
		<List className="min-h-0 flex-1 p-1.5">
			{positions.map((pos) => (
				<button
					type="button"
					data-pos={pos.symbol}
					data-position-card
					key={pos.symbol}
					onClick={() => terminalStore.actions.openThesis(pos.symbol)}
					title="Inspect this lot"
					className="mb-1.25 block w-full cursor-pointer rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 text-left font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
				>
					<Flex.Column className="gap-0">
						<Flex.Row className="items-center justify-between gap-2">
							<Flex.Row className="min-w-0 items-center gap-1.5">
								<Typography.Span className="font-semibold text-[11.5px] text-(--f1)">
									{pos.symbol}
								</Typography.Span>
								<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide">
									{pos.status}
								</Typography.Span>
								<Typography.Span className="text-[8px] uppercase text-(--f4)">
									{pos.stoploss}
								</Typography.Span>
							</Flex.Row>
							<Flex.Row className="items-center gap-1.5">
								<Typography.Span className="text-right font-semibold text-[11.5px] text-(--pnl)">
									{pos.pnl}
								</Typography.Span>
								<button
									type="button"
									disabled={pendingExits.has(pos.symbol)}
									onClick={(event) => {
										event.preventDefault();
										event.stopPropagation();
										requestExit(pos.symbol);
									}}
									title="Exit this position immediately"
									className="rounded-xs border border-(--down) px-1.5 py-px text-[8px] font-semibold text-(--down) uppercase tracking-wide hover:bg-[color-mix(in_srgb,var(--down)_12%,transparent)] disabled:cursor-wait disabled:opacity-60"
								>
									{pendingExits.has(pos.symbol) ? "EXITING" : "EXIT"}
								</button>
							</Flex.Row>
						</Flex.Row>

						<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
							<Typography.Span>
								entry {pos.entryPrice} / mark {pos.mark}
							</Typography.Span>
							<Typography.Span className="text-(--pnl)">
								{pos.returnPct}
							</Typography.Span>
						</Flex.Row>

						<PositionStopGeometry symbol={pos.symbol} />
					</Flex.Column>
				</button>
			))}
		</List>
	);
};