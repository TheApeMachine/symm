import { useSelector } from "@tanstack/react-store";
import { positionsAtom } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import type { PositionT } from "#/providers/telemetry/telemetry/position";
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

type PositionCardData = {
	symbol: string;
	status: string;
	pnl: string;
	pnlValue: number;
	entryPrice: string;
	mark: string;
	returnPct: string;
};

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

		const rawPnl = currentHolding.pnl;
		const pnlNum =
			typeof rawPnl === "number"
				? rawPnl
				: typeof rawPnl === "string" && Number.isFinite(Number(rawPnl))
					? Number(rawPnl)
					: 0;

		result.push({
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
			l.returnPct !== r.returnPct
		) {
			return false;
		}
	}
	return true;
};

export const Positions = () => {
	const positions = useSelector(positionsAtom, selectPositions, {
		compare: positionsEqual,
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
							</Flex.Column>
						</button>
					);
				})
			)}
		</List>
	);
};
