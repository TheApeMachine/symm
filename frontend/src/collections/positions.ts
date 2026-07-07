import { createStore } from "@tanstack/react-store";

export type Position = {
	symbol: string;
	qty: number;
	entry_price: number;
	mark: number;
	pnl: number;
	return_pct: number;
};

type PositionFrame = Record<string, unknown>;

const asFrame = (value: unknown): PositionFrame | null =>
	typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as PositionFrame)
		: null;

const finite = (value: unknown): number | null => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : null;
};

const parsePosition = (value: unknown): Position | null => {
	const frame = asFrame(value);

	if (frame === null || typeof frame.symbol !== "string") {
		return null;
	}

	const qty = finite(frame.qty);
	const entryPrice = finite(frame.entry_price);
	const mark = finite(frame.mark);
	const pnl = finite(frame.pnl);
	const returnPct = finite(frame.return_pct);

	if (
		qty === null ||
		entryPrice === null ||
		mark === null ||
		pnl === null ||
		returnPct === null
	) {
		return null;
	}

	return {
		symbol: frame.symbol,
		qty,
		entry_price: entryPrice,
		mark,
		pnl,
		return_pct: returnPct,
	};
};

export const normalizePositions = (positions: unknown): Position[] => {
	if (!Array.isArray(positions)) {
		console.warn("positionsStore: expected positions array", positions);
		return [];
	}

	const parsed = positions.flatMap((position) => {
		const parsedPosition = parsePosition(position);

		return parsedPosition === null ? [] : [parsedPosition];
	});

	if (parsed.length !== positions.length) {
		console.warn("positionsStore: skipped malformed position rows", {
			received: positions.length,
			accepted: parsed.length,
		});
	}

	return parsed;
};

export const positionsStore = createStore(
	{
		positions: [] as Position[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (positions: unknown) =>
			setState(() => ({
				positions: normalizePositions(positions),
				observed: true,
			})),
	}),
);
