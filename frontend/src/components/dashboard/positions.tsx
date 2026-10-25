import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { positionReadout } from "#/components/book-readout";
import { positionRows } from "#/components/learning/open-positions";
import { formatValue } from "#/components/ui/dom";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { formatExitTrigger } from "#/lib/trigger-labels";
import { cn } from "@/lib/utils";

const formatNumber = (value: unknown, digits: number): string => {
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

const sellMarkers = (
	sells: Record<string, unknown>[] | undefined,
): SellMarker[] =>
	(sells ?? []).map((sell) => {
		const trigger = String(sell.trigger ?? "");
		const qty = typeof sell.qty === "string" && sell.qty !== "" ? sell.qty : "";
		const at = Number(sell.at ?? 0);
		const time = at > 0 ? new Date(at / 1e6).toLocaleTimeString() : "";

		return {
			trigger,
			label: `${formatExitTrigger(trigger)}${qty ? ` ${formatNumber(qty, 4)} qty` : ""}`,
			title: [
				trigger,
				time,
				qty ? `qty ${qty}` : "no order",
				`capacity ${formatNumber(sell.capacityQty, 4)} (ratio ${formatNumber(sell.capacityRatio, 2)})`,
				`budget ${formatNumber(Number(sell.budget ?? 0) * 100, 3)}%`,
				String(sell.note ?? ""),
			]
				.filter((part) => part !== "")
				.join(" · "),
		};
	});

const readPositions = () => positionRows(signals.positions.state);

const selectPositions = (
	rows: Record<string, unknown>[],
): PositionCardData[] => {
	const result: PositionCardData[] = [];

	for (const currentPosition of rows) {
		const currentSymbol =
			typeof currentPosition.symbol === "string" ? currentPosition.symbol : "";
		if (!currentSymbol) continue;

		const readout = positionReadout(currentPosition);

		// Closed lots belong to the closed section below, not the open cards.
		if (readout.closedAt) continue;

		const positionStatus = currentPosition.virtual ? "virtual" : "open";
		const pnlFrac = readout.returnPct ?? 0;

		result.push({
			capacity: "—",
			capacityShort: false,
			capacityRatioNum: undefined,
			budget: currentPosition.path ? String(currentPosition.path) : "—",
			venuePnl: "—",
			shadowPnl: formatValue(pnlFrac, "pct"),
			shadowValue: pnlFrac,
			sells: sellMarkers(undefined),
			symbol: currentSymbol,
			status: positionStatus,
			pnl: formatValue(pnlFrac, "pct"),
			pnlValue: pnlFrac,
			entryPrice: formatNumber(readout.entryPrice, 6),
			mark: formatNumber(readout.mark, 6),
			returnPct: formatValue(pnlFrac, "pct"),
		});
	}

	return result.sort((leftPosition, rightPosition) =>
		leftPosition.symbol.localeCompare(rightPosition.symbol),
	);
};

type ClosedCardData = {
	key: string;
	symbol: string;
	venuePnl: string;
	shadowPnl: string;
	shadowValue: number;
	sells: SellMarker[];
};

const selectClosed = (rows: Record<string, unknown>[]): ClosedCardData[] =>
	rows
		.map((row) => ({ row, readout: positionReadout(row) }))
		.filter(({ readout }) => readout.closedAt !== null)
		.map(({ row, readout }, index) => {
			const pnlFrac = readout.returnPct ?? 0;

			return {
				key: String(row.id ?? row.symbol ?? index),
				symbol: String(row.symbol ?? ""),
				venuePnl: "—",
				shadowPnl: formatValue(pnlFrac, "pct"),
				shadowValue: pnlFrac,
				sells: sellMarkers(undefined),
			};
		})
		.reverse();

const SellMarkers = ({ sells }: { sells: SellMarker[] }) =>
	sells.length === 0 ? null : (
		<Flex.Row className="mt-0.75 flex-wrap items-center gap-1 text-[8.5px]">
			{sells.map((sell, index) => (
				<Typography.Span
					key={index.toString()}
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
	// The store's new state object is the render trigger; the rows decode
	// from the lot frames it holds.
	useSelector(signals.positions, (state) => state);
	const positions = selectPositions(readPositions());
	const closed = selectClosed(readPositions());

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
							onClick={() =>
								terminalStore.actions.selectFocusSymbol(pos.symbol)
							}
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
										) : pos.capacityRatioNum !== undefined &&
											pos.capacityRatioNum < 2 ? (
											<Typography.Span className="rounded-xs bg-(--warn)/15 border border-(--warn)/40 px-1 py-px text-[7.5px] font-bold text-(--warn) uppercase tracking-wide">
												TIGHT
											</Typography.Span>
										) : pos.capacityRatioNum !== undefined &&
											pos.capacityRatioNum >= 2 ? (
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
										className={cn(
											pos.capacityShort && "font-semibold text-(--down)",
										)}
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
