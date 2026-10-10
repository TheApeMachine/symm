import { useSelector } from "@tanstack/react-store";
import { closedPositionsAtom, positionsAtom } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import type { HoldingT } from "#/providers/telemetry/telemetry/holding";
import type { PositionT } from "#/providers/telemetry/telemetry/position";
import type { SellEventT } from "#/providers/telemetry/telemetry/sell-event";
import { cn } from "@/lib/utils";

const formatValue = (value: unknown, digits: number): string => {
	if (typeof value === "number") {
		return value.toFixed(digits);
	}

	if (
		typeof value === "string" &&
		value !== "" &&
		Number.isFinite(Number(value))
	) {
		return Number(value).toFixed(digits);
	}

	return String(value ?? "—");
};

const pnlTone = (value: number): "up" | "down" | "f3" => {
	if (value > 0) {
		return "up";
	}

	if (value < 0) {
		return "down";
	}

	return "f3";
};

type SellMarker = {
	trigger: string;
	label: string;
	title: string;
};

type PositionCardData = {
	symbol: string;
	status: string;
	pnl: string;
	pnlValue: number;
	entryPrice: string;
	mark: string;
	returnPct: string;
	capacity: string;
	capacityShort: boolean;
	capacityRatioNum?: number;
	budget: string;
	venuePnl: string;
	shadowPnl: string;
	shadowValue: number;
	sells: SellMarker[];
};

/*
TRIGGER_LABEL abbreviates each sell trigger for its marker. The learned exit
is the primary exit; capacity trims and exits come from the risk monitor.
*/
const TRIGGER_LABEL: Record<string, string> = {
	learned_exit: "EXIT",
	capacity_trim: "TRIM",
	capacity_exit: "CAP-EXIT",
	manual: "MANUAL",
};

const sellMarkers = (sells: SellEventT[] | undefined): SellMarker[] =>
	(sells ?? []).map((sell) => {
		const trigger = String(sell.trigger ?? "");
		const qty = typeof sell.qty === "string" && sell.qty !== "" ? sell.qty : "";
		const at = Number(sell.at ?? 0n);
		const time = at > 0 ? new Date(at / 1e6).toLocaleTimeString() : "";

		return {
			trigger,
			label: `${TRIGGER_LABEL[trigger] ?? trigger}${qty ? ` ${formatValue(qty, 4)} qty` : ""}`,
			title: [
				trigger,
				time,
				qty ? `qty ${qty}` : "no order",
				`capacity ${formatValue(sell.capacityQty, 4)} (ratio ${formatValue(sell.capacityRatio, 2)})`,
				`budget ${formatValue(Number(sell.budget ?? 0) * 100, 3)}%`,
				String(sell.note ?? ""),
			]
				.filter((part) => part !== "")
				.join(" · "),
		};
	});

const numeric = (value: unknown): number =>
	typeof value === "number"
		? value
		: typeof value === "string" && value !== "" && Number.isFinite(Number(value))
			? Number(value)
			: 0;

/*
pnlPair renders venue P&L (paper fills) beside shadow P&L (every fill walked
through the as-of book), the evaluation truth; shadow is "—" when undefined.
*/
const pnlPair = (holding: HoldingT) => ({
	venuePnl: holding.venuePnl ? formatValue(holding.venuePnl, 4) : "—",
	shadowPnl:
		holding.shadowDefined && holding.shadowPnl
			? formatValue(holding.shadowPnl, 4)
			: "—",
	shadowValue: holding.shadowDefined ? numeric(holding.shadowPnl) : 0,
});

const selectPositions = (rows: PositionT[]): PositionCardData[] => {
	const result: PositionCardData[] = [];

	for (const currentPosition of rows) {
		const currentHolding = currentPosition.holding;
		if (!currentHolding) continue;

		const currentSymbol =
			typeof currentHolding.symbol === "string" ? currentHolding.symbol : "";
		if (!currentSymbol) continue;

		const rawStatus = currentHolding.status ?? currentPosition.status;
		const positionStatus =
			typeof rawStatus === "string" ? rawStatus : "—";
		if (positionStatus === "closed") continue;

		const pnlNum = numeric(currentHolding.pnl);
		const ratio = Number(currentHolding.capacityRatio ?? 0);

		result.push({
			capacity: currentHolding.capacityDefined
				? `${formatValue(ratio, 2)}×${currentHolding.capacityBounded ? "+" : ""}`
				: "—",
			capacityShort: Boolean(currentHolding.capacityDefined) && ratio < 1,
			capacityRatioNum: currentHolding.capacityDefined ? ratio : undefined,
			budget: currentHolding.budgetSource
				? `${formatValue(Number(currentHolding.slippageBudget ?? 0) * 100, 3)}% ${currentHolding.budgetSource}`
				: "—",
			...pnlPair(currentHolding),
			sells: sellMarkers(currentHolding.sells),
			symbol: currentSymbol,
			status: positionStatus,
			pnl: `${formatValue(currentHolding.pnl, 4)} USD`,
			pnlValue: pnlNum,
			entryPrice: formatValue(currentHolding.entryPrice, 6),
			mark: formatValue(currentHolding.mark, 6),
			returnPct: `${formatValue(currentHolding.returnPct, 2)}%`,
		});
	}

	return result.sort((leftPosition, rightPosition) =>
		leftPosition.symbol.localeCompare(rightPosition.symbol),
	);
};

const positionsEqual = (
	left: PositionCardData[],
	right: PositionCardData[],
): boolean => {
	if (left === right) return true;
	if (left.length !== right.length) return false;
	for (let index = 0; index < left.length; index++) {
		const l = left[index];
		const r = right[index];
		if (
			l.symbol !== r.symbol ||
			l.status !== r.status ||
			l.pnl !== r.pnl ||
			l.pnlValue !== r.pnlValue ||
			l.entryPrice !== r.entryPrice ||
			l.mark !== r.mark ||
			l.returnPct !== r.returnPct ||
			l.capacity !== r.capacity ||
			l.capacityRatioNum !== r.capacityRatioNum ||
			l.budget !== r.budget ||
			l.venuePnl !== r.venuePnl ||
			l.shadowPnl !== r.shadowPnl ||
			l.sells.length !== r.sells.length
		) {
			return false;
		}
	}
	return true;
};

type ClosedCardData = {
	key: string;
	symbol: string;
	venuePnl: string;
	shadowPnl: string;
	shadowValue: number;
	sells: SellMarker[];
};

const selectClosed = (closed: HoldingT[]): ClosedCardData[] =>
	closed
		.map((holding, index) => ({
			key: `${holding.symbol}-${String(holding.closedAt ?? index)}`,
			symbol: String(holding.symbol ?? ""),
			...pnlPair(holding),
			sells: sellMarkers(holding.sells),
		}))
		.reverse();

const closedEqual = (left: ClosedCardData[], right: ClosedCardData[]) =>
	left.length === right.length &&
	left.every((card, index) => card.key === right[index]?.key);

const SellMarkers = ({ sells }: { sells: SellMarker[] }) =>
	sells.length === 0 ? null : (
		<Flex.Row className="mt-0.75 flex-wrap items-center gap-1 text-[8.5px]">
			{sells.map((sell, index) => (
				<Typography.Span
					// biome-ignore lint/suspicious/noArrayIndexKey: sells are append-only
					key={index}
					data-trigger={sell.trigger}
					title={sell.title}
					className={cn(
						"rounded-xs border px-1 py-px uppercase tracking-wide",
						sell.trigger === "learned_exit" && "border-(--acc) text-(--acc)",
						sell.trigger !== "learned_exit" && "border-(--down) text-(--down)",
					)}
				>
					{sell.label}
				</Typography.Span>
			))}
		</Flex.Row>
	);

const PnlPair = ({
	venuePnl,
	shadowPnl,
	shadowValue,
}: {
	venuePnl: string;
	shadowPnl: string;
	shadowValue: number;
}) => {
	const tone = pnlTone(shadowValue);

	return (
		<Typography.Span>
			venue {venuePnl} /{" "}
			<span
				className={cn(
					"font-semibold",
					tone === "up" && "text-(--up)",
					tone === "down" && "text-(--down)",
				)}
			>
				shadow {shadowPnl}
			</span>
		</Typography.Span>
	);
};

export const Positions = () => {
	const positions = useSelector(positionsAtom, selectPositions, {
		compare: positionsEqual,
	});
	const closed = useSelector(closedPositionsAtom, selectClosed, {
		compare: closedEqual,
	});

	return (
		<List className="min-h-0 flex-1 p-1.5">
			{positions.length === 0 ? (
				<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
					no open positions
				</div>
			) : (
				positions.map((pos) => {
					const tone = pnlTone(pos.pnlValue);
					return (
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
										{pos.capacityShort ? (
											<Typography.Span className="rounded-xs bg-(--down)/15 border border-(--down)/40 px-1 py-px text-[7.5px] font-bold text-(--down) uppercase tracking-wide">
												SHORTFALL
											</Typography.Span>
										) : pos.capacityRatioNum !== undefined && pos.capacityRatioNum < 2 ? (
											<Typography.Span className="rounded-xs bg-(--warn)/15 border border-(--warn)/40 px-1 py-px text-[7.5px] font-bold text-(--warn) uppercase tracking-wide">
												TIGHT
											</Typography.Span>
										) : pos.capacityRatioNum !== undefined && pos.capacityRatioNum >= 2 ? (
											<Typography.Span className="rounded-xs bg-(--up)/15 border border-(--up)/40 px-1 py-px text-[7.5px] font-bold text-(--up) uppercase tracking-wide">
												LIQUID
											</Typography.Span>
										) : null}
									</Flex.Row>
									<Typography.Span
										className={cn(
											"text-right font-semibold text-[11.5px]",
											tone === "up" && "text-(--up)",
											tone === "down" && "text-(--down)",
											tone === "f3" && "text-(--f2)",
										)}
									>
										{pos.pnl}
									</Typography.Span>
								</Flex.Row>

								<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
									<Typography.Span>
										entry {pos.entryPrice} / mark {pos.mark}
									</Typography.Span>
									<Typography.Span
										className={cn(
											"font-semibold",
											tone === "up" && "text-(--up)",
											tone === "down" && "text-(--down)",
											tone === "f3" && "text-(--f2)",
										)}
									>
										{pos.returnPct}
									</Typography.Span>
								</Flex.Row>

								<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
									<Typography.Span
										data-capacity
										title="exit capacity within the slippage budget / open quantity (+ = lower bound)"
										className={cn(pos.capacityShort && "font-semibold text-(--down)")}
									>
										capacity {pos.capacity}
									</Typography.Span>
									<Typography.Span title="per-side slippage budget and its source">
										budget {pos.budget}
									</Typography.Span>
								</Flex.Row>

								{pos.capacityRatioNum !== undefined && (
									<div
										className="mt-1 w-full bg-(--surface) border border-(--line)/40 rounded-full h-1 overflow-hidden"
										title={`Liquidity capacity: ${pos.capacityRatioNum.toFixed(2)}x open position`}
									>
										<div
											className={cn(
												"h-full transition-all duration-300",
												pos.capacityShort
													? "bg-(--down)"
													: pos.capacityRatioNum < 2
														? "bg-(--warn)"
														: "bg-(--up)",
											)}
											style={{
												width: `${Math.min(100, Math.max(8, (pos.capacityRatioNum / 3) * 100))}%`,
											}}
										/>
									</div>
								)}

								<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
									<PnlPair
										venuePnl={pos.venuePnl}
										shadowPnl={pos.shadowPnl}
										shadowValue={pos.shadowValue}
									/>
								</Flex.Row>

								<SellMarkers sells={pos.sells} />
							</Flex.Column>
						</button>
					);
				})
			)}

			{closed.length === 0 ? null : (
				<div data-closed className="mt-2 border-(--line) border-t pt-1.5">
					<div className="px-1 pb-1 font-mono text-[9px] text-(--f4) uppercase tracking-wide">
						closed this session
					</div>
					{closed.map((card) => (
						<div
							key={card.key}
							data-closed-card={card.symbol}
							className="mb-1.25 rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 font-mono text-[10px]"
						>
							<Flex.Row className="items-center justify-between gap-2">
								<Typography.Span className="font-semibold text-(--f1)">
									{card.symbol}
								</Typography.Span>
								<PnlPair
									venuePnl={card.venuePnl}
									shadowPnl={card.shadowPnl}
									shadowValue={card.shadowValue}
								/>
							</Flex.Row>
							<SellMarkers sells={card.sells} />
						</div>
					))}
				</div>
			)}
		</List>
	);
};
