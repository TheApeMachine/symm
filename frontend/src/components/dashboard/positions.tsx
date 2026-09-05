import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { positionStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { sendPositionExit } from "#/providers/websocket";

const formatValue = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "string" &&
				value !== "" &&
				Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

const positionObject = new Position();
const holdingObject = new Holding();

type PositionCardData = {
	symbol: string;
	status: string;
	pnl: string;
	entryPrice: string;
	mark: string;
	returnPct: string;
};

export const Positions = () => {
	const positions = useSelector(positionStore, (state) => {
		const latestFrame = state.findLast(() => true);
		if (!latestFrame) return [];

		const currentPositions: PositionCardData[] = [];

		for (let rowIndex = 0; rowIndex < latestFrame.rowsLength(); rowIndex++) {
			const currentPosition = latestFrame.rows(rowIndex, positionObject);
			if (!currentPosition) continue;

			const currentHolding = currentPosition.holding(holdingObject);
			if (!currentHolding) continue;

			const currentSymbol = currentHolding.symbol() ?? "";
			if (!currentSymbol) continue;

			const positionStatus =
				currentHolding.status() ?? currentPosition.status() ?? "—";
			if (positionStatus === "closed") {
				continue;
			}

			currentPositions.push({
				symbol: currentSymbol,
				status: positionStatus,
				pnl: `${formatValue(currentHolding.pnl(), 4)} USD`,
				entryPrice: formatValue(currentHolding.entryPrice(), 6),
				mark: formatValue(currentHolding.mark(), 6),
				returnPct: `${formatValue(currentHolding.returnPct(), 2)}%`,
			});
		}

		return currentPositions.sort((leftPosition, rightPosition) =>
			leftPosition.symbol.localeCompare(rightPosition.symbol),
		);
	});
	const [pendingExits, setPendingExits] = useState<ReadonlySet<string>>(
		new Set(),
	);

	// A closed lot drops out of `positions` entirely, so its pending flag would
	// otherwise linger forever — clear it the moment the symbol is no longer
	// open.
	useEffect(() => {
		const openSymbols = new Set(positions.map((pos) => pos.symbol));

		setPendingExits((current) => {
			const next = new Set(
				[...current].filter((symbol) => openSymbols.has(symbol)),
			);
			return next.size === current.size ? current : next;
		});
	}, [positions]);

	const requestExit = (symbol: string) => {
		if (pendingExits.has(symbol)) {
			return;
		}

		setPendingExits((current) => new Set(current).add(symbol));
		sendPositionExit(symbol);

		// The exit command has no ack on this socket — if the desk rejects or
		// silently drops it, the button must not stay disabled forever with no
		// way to retry.
		setTimeout(() => {
			setPendingExits((current) => {
				if (!current.has(symbol)) return current;
				const next = new Set(current);
				next.delete(symbol);
				return next;
			});
		}, 10_000);
	};

	return (
		<List className="min-h-0 flex-1 p-1.5">
			{positions.map((pos) => (
				// biome-ignore lint/a11y/useSemanticElements: a <button> can't legally nest the EXIT <button>.
				<div
					role="button"
					tabIndex={0}
					data-pos={pos.symbol}
					data-position-card
					key={pos.symbol}
					onClick={() => terminalStore.actions.openThesis(pos.symbol)}
					onKeyDown={(event) => {
						if (event.key === "Enter" || event.key === " ") {
							event.preventDefault();
							terminalStore.actions.openThesis(pos.symbol);
						}
					}}
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
					</Flex.Column>
				</div>
			))}
		</List>
	);
};
