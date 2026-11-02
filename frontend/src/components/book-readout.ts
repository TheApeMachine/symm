import type { Book } from "#/collections/book";

const decimalPattern = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;

export const decimal = (value: unknown): number | null => {
	if (typeof value === "number") return value;

	if (typeof value !== "string") return null;

	const text = value.trim();
	if (!decimalPattern.test(text)) return null;

	return Number(text);
};

const dateText = (value: unknown): string | null => {
	if (typeof value !== "string" || value === "") return null;

	try {
		return new Date(value).toISOString();
	} catch {
		return null;
	}
};

const returnFraction = (row: Record<string, unknown>) => {
	const pnlPct = decimal(row.pnl_pct);
	if (pnlPct !== null) return pnlPct / 100;
	return null;
};

const outcomeTriggers = (value: unknown): string[] | null => {
	if (!Array.isArray(value)) return null;
	const list = value.filter((item): item is string => typeof item === "string");
	return list.length > 0 ? list : null;
};

export type SettledOutcome = {
	pnl: number | null;
	pnl_pct: number | null;
	hold_ns: number | null;
	triggers: string[] | null;
};

export const settledOutcome = (
	row: Record<string, unknown>,
): SettledOutcome | null => {
	const pnlPct = decimal(row.pnl_pct);
	const frac = returnFraction(row);

	if (frac === null && pnlPct === null) return null;

	const openedAt = dateText(row.opened_at);
	const closedAt = dateText(row.closed_at);
	const holdNs =
		closedAt !== null && openedAt !== null
			? (Date.parse(closedAt) - Date.parse(openedAt)) * 1e6
			: null;

	return {
		pnl: frac,
		pnl_pct: pnlPct ?? (frac === null ? null : frac * 100),
		hold_ns: holdNs,
		triggers: outcomeTriggers(row.outcome_triggers),
	};
};

type ExecutionTotals = {
	qty: number | null;
	cost: number | null;
	fee: number | null;
	vwap: number | null;
	firstAt: string | null;
	lastAt: string | null;
};

const executionTotals = (
	row: Record<string, unknown>,
	side: "buy" | "sell",
): ExecutionTotals => {
	let qty = 0;
	let cost = 0;
	let fee = 0;
	let priceQty = 0;
	let priceNotional = 0;
	let sawFill = false;
	let costKnown = true;
	let feeKnown = true;
	let firstAt: string | null = null;
	let lastAt: string | null = null;
	const seen = new Set<string>();

	for (const fill of (row.fills as Record<string, unknown>[]) ?? []) {
		if (typeof fill.side !== "string" || fill.side.toLowerCase() !== side) {
			continue;
		}

		const identity = fill.exec_id ?? fill.trade_id;
		if (identity !== undefined && identity !== null) {
			const key = String(identity);
			if (seen.has(key)) continue;
			seen.add(key);
		}

		const fillQty = decimal(fill.last_qty) ?? decimal(fill.cum_qty);
		if (fillQty === null || fillQty <= 0) continue;

		const fillCost = decimal(fill.cost) ?? decimal(fill.cum_cost);
		const fillFee = decimal(fill.fee_usd_equiv);
		const fillPrice =
			decimal(fill.avg_price) ?? decimal(fill.last_price);
		const fillAt = dateText(fill.timestamp);

		qty += fillQty;
		sawFill = true;

		if (fillCost === null) {
			costKnown = false;
		} else {
			cost += fillCost;
		}

		if (fillFee === null) {
			feeKnown = false;
		} else {
			fee += fillFee;
		}

		if (fillPrice !== null) {
			priceQty += fillQty;
			priceNotional += fillPrice * fillQty;
		}

		firstAt ??= fillAt;
		lastAt = fillAt ?? lastAt;
	}

	const vwap =
		costKnown && qty > 0
			? cost / qty
			: priceQty > 0
				? priceNotional / priceQty
				: null;

	return {
		qty: sawFill ? qty : null,
		cost: sawFill && costKnown ? cost : null,
		fee: sawFill && feeKnown ? fee : null,
		vwap,
		firstAt,
		lastAt,
	};
};

const orderVolume = (order: unknown): number | null => {
	if (!order || typeof order !== "object") return null;
	return decimal((order as Record<string, unknown>).volume);
};

export type PositionReadout = {
	qty: number | null;
	entryQty: number | null;
	entryCost: number | null;
	exitQty: number | null;
	entryPrice: number | null;
	mark: number | null;
	entryFee: number | null;
	exitFee: number | null;
	fees: number | null;
	exitVwap: number | null;
	exitRoom: number | null;
	returnPct: number | null;
	returnPctDisplay: number | null;
	openedAt: string | null;
	closedAt: string | null;
	holdNs: number | null;
	closed: boolean;
	exitPending: boolean;
};

export const positionReadout = (
	row: Record<string, unknown>,
): PositionReadout => {
	const buys = executionTotals(row, "buy");
	const sells = executionTotals(row, "sell");
	const orderedQty = orderVolume(row.entry_order);
	const qty = buys.qty ?? orderedQty;
	const entryPrice = buys.vwap;
	const mark = decimal(row.mark);
	const exitVwap = decimal(row.exit_vwap) ?? sells.vwap;
	const exitRoom = decimal(row.exit_room);
	const entryFee = buys.fee;
	const exitFee = sells.fee;
	const fees =
		entryFee !== null || exitFee !== null
			? (entryFee ?? 0) + (exitFee ?? 0)
			: null;
	const openedAt = dateText(row.opened_at) ?? buys.firstAt;
	const closedAt = dateText(row.closed_at);
	const fullySold =
		buys.qty !== null && sells.qty !== null && sells.qty >= buys.qty;
	const closed = closedAt !== null || fullySold;
	const holdNs =
		closedAt !== null && openedAt !== null
			? (Date.parse(closedAt) - Date.parse(openedAt)) * 1e6
			: null;

	const returnPct = returnFraction(row);
	const returnPctDisplay = decimal(row.pnl_pct);

	return {
		qty,
		entryQty: buys.qty ?? orderedQty,
		entryCost: buys.cost,
		exitQty: sells.qty,
		entryPrice,
		mark,
		entryFee,
		exitFee,
		fees,
		exitVwap,
		exitRoom,
		returnPct,
		returnPctDisplay,
		openedAt,
		closedAt: closedAt ?? (fullySold ? sells.lastAt : null),
		holdNs,
		closed,
		exitPending: row.exit_order != null && !closed,
	};
};

/** Close-now return in display percent — shared by position Trail and open-choices chart. */
export const closeNowReturnPct = (
	row: Record<string, unknown>,
): number | null => {
	const readout = positionReadout(row);
	return (
		readout.returnPctDisplay ??
		(readout.returnPct === null ? null : readout.returnPct * 100)
	);
};

/*
Match a desk row to the selected ledger. Lots without a book label (not yet
on the wire) default to paper so virtual/real tabs stay empty until labeled.
*/
export const onBook = (row: Record<string, unknown>, book: Book) => {
	if (typeof row.book === "string" && row.book !== "") {
		return row.book === book;
	}

	return book === "paper";
};

export const openOnBook = (rows: Record<string, unknown>[], book: Book) =>
	rows.filter((row) => onBook(row, book) && !positionReadout(row).closed);

export const closedOnBook = (rows: Record<string, unknown>[], book: Book) =>
	rows.filter((row) => onBook(row, book) && positionReadout(row).closed);

export const countsTowardBookPerformance = (
	row: Record<string, unknown>,
	book: Book,
): boolean => onBook(row, book);
