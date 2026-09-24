import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { type OpenPosition, PositionList } from "#/components/ui/position-list";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { sendPositionExit } from "#/providers/websocket";

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

const positionObject = new Position();
const holdingObject = new Holding();

export const selectPositions = (state: any): OpenPosition[] => {
	const latestFrame =
		typeof state?.findLast === "function"
			? state.findLast(() => true)
			: Array.isArray(state)
				? state[state.length - 1]
				: state;
	if (!latestFrame || typeof latestFrame.rowsLength !== "function") {
		return [];
	}

	const fallbackPositions: OpenPosition[] = [];

	for (let rowIndex = 0; rowIndex < latestFrame.rowsLength(); rowIndex++) {
		const currentPosition = latestFrame.rows(rowIndex, positionObject);
		if (!currentPosition) {
			continue;
		}

		const currentHolding = currentPosition.holding(holdingObject);
		if (!currentHolding) {
			continue;
		}

		const currentSymbol = currentHolding.symbol() ?? "";
		if (!currentSymbol) {
			continue;
		}

		const positionStatus =
			currentHolding.status() ?? currentPosition.status() ?? "—";
		if (positionStatus === "closed") {
			continue;
		}

		const rawPnl = currentHolding.pnl();
		const pnlNum =
			typeof rawPnl === "number"
				? rawPnl
				: typeof rawPnl === "string" && Number.isFinite(Number(rawPnl))
					? Number(rawPnl)
					: 0;

		fallbackPositions.push({
			symbol: currentSymbol,
			status: positionStatus,
			pnl: `${formatValue(currentHolding.pnl(), 4)} USD`,
			pnlValue: pnlNum,
			entryPrice: formatValue(currentHolding.entryPrice(), 6),
			mark: formatValue(currentHolding.mark(), 6),
			returnPct: `${formatValue(currentHolding.returnPct(), 2)}%`,
		});
	}

	return fallbackPositions.sort((leftPosition, rightPosition) =>
		leftPosition.symbol.localeCompare(rightPosition.symbol),
	);
};

export const positionsEqual = (
	left: OpenPosition[],
	right: OpenPosition[],
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

/*
Positions binds the desk to the list that draws it. The lots, which of them are
on their way out, and what a click means all live here; the drawing does not.
*/
export const Positions = () => {
	const positions = useSelector(signals.position, selectPositions, {
		compare: positionsEqual,
	});
	const [pendingExits, setPendingExits] = useState<ReadonlySet<string>>(
		new Set(),
	);

	// A closed lot drops out of `positions` entirely, so its pending flag would
	// otherwise linger forever — clear it the moment the symbol is no longer
	// open.
	useEffect(() => {
		if (pendingExits.size === 0) return;

		const openSymbols = new Set(positions.map((pos) => pos.symbol));
		const next = new Set(
			[...pendingExits].filter((symbol) => openSymbols.has(symbol)),
		);
		if (next.size !== pendingExits.size) {
			setPendingExits(next);
		}
	}, [positions, pendingExits]);

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
		<PositionList
			positions={positions}
			exiting={[...pendingExits]}
			onInspect={(symbol) => terminalStore.actions.openThesis(symbol)}
			onExit={requestExit}
		/>
	);
};
