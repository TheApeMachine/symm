import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";

const formatNumber = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "string" && value !== "" && Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

const positionHolder = new Position();
const holdingHolder = new Holding();
const stoplossHolder = new Stoploss();

type ActiveLotEntry = {
	symbol: string;
	status: string;
	stopStatus: string;
	pnl: string;
	mark: string;
	returnPct: string;
	entryPrice: string;
	entryAt: string;
	floor: string;
	peak: string;
	locked: boolean;
	surgeArmed: boolean;
};

type JournalTradeEntry = {
	id: string;
	symbol: string;
	status: string;
	pnl: string;
	returnPct: string;
	entryPrice: string;
	entryAt: string;
	exitPrice: string;
	entryFee: string;
	exitFee: string;
	triggerReason: string;
	stopStatus: string;
	locked: boolean;
	surgeArmed: boolean;
	exitAt: string;
};

/*
JournalSurface is the record of what the desk actually did.
*/
export const JournalSurface = () => {
	const { activeLots, closedTrades } = useSelector(positionStore, (state) => {
		const activeMap = new Map<string, ActiveLotEntry>();
		const closedMap = new Map<string, JournalTradeEntry>();

		for (const frame of state.toArray()) {
			for (let rowIndex = 0; rowIndex < frame.rowsLength(); rowIndex++) {
				const currentPosition = frame.rows(rowIndex, positionHolder);
				if (!currentPosition) continue;

				const currentHolding = currentPosition.holding(holdingHolder);
				if (!currentHolding) continue;

				const currentSymbol = currentHolding.symbol() ?? "";
				if (!currentSymbol) continue;

				const positionStatus = currentHolding.status() ?? currentPosition.status() ?? "—";
				const currentStoploss = currentHolding.stoploss(stoplossHolder);

				const entryNano = currentHolding.entryAt();
				const entryTimestamp = entryNano > 0n
					? new Date(Number(entryNano / 1000000n)).toLocaleTimeString()
					: "—";

				if (positionStatus === "closed" || currentHolding.exitPrice()) {
					activeMap.delete(currentSymbol);

					const exitNano = currentHolding.exitAt();
					const exitTimestamp = exitNano > 0n
						? new Date(Number(exitNano / 1000000n)).toLocaleTimeString()
						: "—";

					const tradeId = `${currentSymbol}-${String(exitNano)}`;
					closedMap.set(tradeId, {
						id: tradeId,
						symbol: currentSymbol,
						status: positionStatus,
						pnl: `${formatNumber(currentHolding.pnl(), 4)} USD`,
						returnPct: `${formatNumber(currentHolding.returnPct(), 2)}%`,
						entryPrice: formatNumber(currentHolding.entryPrice(), 6),
						entryAt: entryTimestamp,
						exitPrice: formatNumber(currentHolding.exitPrice(), 6),
						entryFee: formatNumber(currentHolding.entryFee(), 4),
						exitFee: formatNumber(currentHolding.exitFee(), 4),
						triggerReason: currentStoploss?.triggerReason() ?? "—",
						stopStatus: currentStoploss?.status() ?? "—",
						locked: currentStoploss?.locked() ?? false,
						surgeArmed: currentStoploss?.surgeArmed() ?? false,
						exitAt: exitTimestamp,
					});
					continue;
				}

				activeMap.set(currentSymbol, {
					symbol: currentSymbol,
					status: positionStatus,
					stopStatus: currentStoploss?.status() ?? "—",
					pnl: `${formatNumber(currentHolding.pnl(), 4)} USD`,
					mark: formatNumber(currentHolding.mark(), 6),
					returnPct: `${formatNumber(currentHolding.returnPct(), 2)}%`,
					entryPrice: formatNumber(currentHolding.entryPrice(), 6),
					entryAt: entryTimestamp,
					floor: formatNumber(currentStoploss?.floor(), 6),
					peak: formatNumber(currentStoploss?.peak(), 6),
					locked: currentStoploss?.locked() ?? false,
					surgeArmed: currentStoploss?.surgeArmed() ?? false,
				});
			}
		}

		return {
			activeLots: [...activeMap.values()].sort((leftLot, rightLot) =>
				leftLot.symbol.localeCompare(rightLot.symbol),
			),
			closedTrades: [...closedMap.values()],
		};
	});

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<Section>
					<Section.Header title="Lifecycle rail" meta={`${activeLots.length} lots`} />
					<div className="flex flex-col gap-2 p-2">
						{activeLots.length === 0 ? (
							<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
								no lots held
							</Panel>
						) : (
							activeLots.map((lot) => (
								<Panel key={lot.symbol} variant="surface" size="bare" className="flex flex-col gap-1 px-2.5 py-2 font-mono text-[11px]">
									<Flex.Row className="items-center justify-between gap-2">
										<Flex.Column className="min-w-0 gap-0.5">
											<Typography.Span className="truncate font-semibold text-(--f1)">
												{lot.symbol}
											</Typography.Span>
											<Typography.Span className="text-[9.5px] text-(--f4)">
												entry {lot.entryPrice} · mark {lot.mark}
											</Typography.Span>
										</Flex.Column>
										<Flex.Column className="shrink-0 items-end gap-0.5">
											<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide">
												{lot.status}
											</Typography.Span>
											<Typography.Span className="text-[9.5px] text-(--pnl)">
												{lot.pnl} ({lot.returnPct})
											</Typography.Span>
										</Flex.Column>
									</Flex.Row>
									<Flex.Row className="items-center justify-between gap-2 border-(--line) border-t pt-1 text-[9px] text-(--f4)">
										<Typography.Span
											className={lot.stopStatus === "error" ? "font-semibold uppercase text-(--down)" : "uppercase"}
										>
											{lot.stopStatus === "error" ? "⚠ stop error" : `stop ${lot.stopStatus}`}
										</Typography.Span>
										<Typography.Span>
											floor {lot.floor} · peak {lot.peak}
										</Typography.Span>
									</Flex.Row>
									<Flex.Row className="items-center justify-between gap-2 text-[9px] text-(--f4)">
										<Typography.Span>entered {lot.entryAt}</Typography.Span>
										<Typography.Span>
											{lot.locked ? "locked" : "unlocked"}
											{lot.surgeArmed ? " · surge" : ""}
										</Typography.Span>
									</Flex.Row>
								</Panel>
							))
						)}
					</div>
				</Section>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-4.5">
				<Section.Header size="bare" rule={false} title="Journal" meta={`${closedTrades.length} entries`} />
				<div className="mt-2 flex flex-col gap-2">
					{closedTrades.length === 0 ? (
						<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
							nothing traded yet this run
						</Panel>
					) : (
						closedTrades.map((trade) => (
							<Panel key={trade.id} variant="surface" size="bare" className="flex flex-col gap-1.5 p-3 font-mono text-[11px]">
								<Flex.Row className="items-center justify-between gap-2">
									<Flex.Row className="items-center gap-2">
										<Typography.Span className="font-semibold text-[12px] text-(--f1)">
											{trade.symbol}
										</Typography.Span>
										<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8.5px] uppercase text-(--f3)">
											{trade.triggerReason}
										</Typography.Span>
									</Flex.Row>
									<Flex.Row className="items-center gap-2">
										<Typography.Span className="font-semibold text-(--pnl)">
											{trade.pnl}
										</Typography.Span>
										<Typography.Span className="text-[9.5px] text-(--pnl)">
											({trade.returnPct})
										</Typography.Span>
									</Flex.Row>
								</Flex.Row>
								<Flex.Row className="items-center justify-between text-[9.5px] text-(--f4)">
									<Typography.Span>
										{trade.entryPrice} → {trade.exitPrice}
									</Typography.Span>
									<Typography.Span>
										entered {trade.entryAt} · exited {trade.exitAt}
									</Typography.Span>
								</Flex.Row>
								<Flex.Row className="items-center justify-between border-(--line) border-t pt-1 text-[9px] text-(--f4)">
									<Typography.Span>
										fees {trade.entryFee} → {trade.exitFee}
									</Typography.Span>
									<Typography.Span>
										<Typography.Span
											className={trade.stopStatus === "error" ? "font-semibold uppercase text-(--down)" : "uppercase"}
										>
											{trade.stopStatus === "error" ? "⚠ stop error" : trade.stopStatus}
										</Typography.Span>
										{trade.locked ? " · locked" : ""}
										{trade.surgeArmed ? " · surge" : ""}
									</Typography.Span>
								</Flex.Row>
							</Panel>
						))
					)}
				</div>
			</div>
		</div>
	);
};