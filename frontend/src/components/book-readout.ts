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

const returnFraction = (row: Record<string, unknown>, closed: boolean) => {
	if (closed) {
		const settled = decimal(row.net_return ?? row.return ?? null);
		return settled === null ? null : settled / 100;
	}

	const open = decimal(row.book_return);
	return open === null ? null : open / 100;
};

/* The desk's settled outcome, when it has priced the close. */
export const settledOutcome = (row: Record<string, unknown>) => {
	if (row.cost === undefined || row.proceeds === undefined) return null;

	const cost = decimal(row.cost);
	const proceeds = decimal(row.proceeds);

	if (cost === null || proceeds === null) return null;

	return {
		cost,
		proceeds,
		fees: null,
		realized: decimal(row.profit) ?? proceeds - cost,
		net_return: row.net_return ?? row.return ?? null,
		hold_ns: row.hold_ns ?? null,
		triggers: row.outcome_triggers ?? null,
	};
};

type ExecutionTotals = {
	qty: number | null;
	cost: number | null;
	firstAt: string | null;
	lastAt: string | null;
};

const executionTotals = (
	row: Record<string, unknown>,
	side: "buy" | "sell",
): ExecutionTotals => {
	let qty = 0;
	let cost = 0;
	let sawFill = false;
	let costKnown = true;
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

		const fillQty = decimal(fill.last_qty);
		if (fillQty === null) continue;

		const fillCost = decimal(fill.cost);
		const fillAt = dateText(fill.timestamp);

		qty += fillQty;
		sawFill = true;

		if (fillCost === null) {
			costKnown = false;
		} else {
			cost += fillCost;
		}

		firstAt ??= fillAt;
		lastAt = fillAt ?? lastAt;
	}

	return {
		qty: sawFill ? qty : null,
		cost: sawFill && costKnown ? cost : null,
		firstAt,
		lastAt,
	};
};

export type PositionReadout = {
	qty: number | null;
	entryQty: number | null;
	entryCost: number | null;
	exitQty: number | null;
	exitProceeds: number | null;
	entryPrice: number | null;
	mark: number | null;
	entryFee: number | null;
	exitFee: number | null;
	fees: number | null;
	exitVwap: number | null;
	exitRoom: number | null;
	grossPnl: number | null;
	netPnl: number | null;
	unrealizedPnl: number | null;
	returnPct: number | null;
	openedAt: string | null;
	closedAt: string | null;
	holdNs: number | null;
	closed: boolean;
	exitPending: boolean;
};

/*
Position economics come from the desk row (broker/price.go). Fills only
help detect venue round trips when closed_at is absent.
*/
export const positionReadout = (
	row: Record<string, unknown>,
): PositionReadout => {
	const buys = executionTotals(row, "buy");
	const sells = executionTotals(row, "sell");
	const qty = decimal(row.qty);
	const entryPrice = decimal(row.entry_price);
	const mark = decimal(row.mark);
	const exitVwap = decimal(row.exit_vwap);
	const exitRoom = decimal(row.exit_room);
	const entryFee = decimal(row.entry_fee);
	const exitFee = decimal(row.exit_fee);
	const openedAt = dateText(row.opened_at) ?? buys.firstAt;
	const fullySold =
		buys.qty !== null && sells.qty !== null && sells.qty >= buys.qty;

	const closed =
		row.closed_at !== null && row.closed_at !== undefined
			? row.virtual !== true ||
				(decimal(row.cost) !== null && decimal(row.proceeds) !== null)
			: fullySold;

	const closedAt = dateText(row.closed_at) ?? (fullySold ? sells.lastAt : null);
	const holdNs =
		row.hold_ns ??
		(closedAt !== null && openedAt !== null
			? (Date.parse(closedAt) - Date.parse(openedAt)) * 1e6
			: null);

	const settledCost = decimal(row.cost);
	const settledProceeds = decimal(row.proceeds);
	const profit = decimal(row.profit);
	const returnPct = returnFraction(row, closed);

	let netPnl: number | null = null;
	let grossPnl: number | null = null;
	let unrealizedPnl: number | null = null;

	if (closed && settledCost !== null && settledProceeds !== null) {
		grossPnl = settledProceeds - settledCost;
		netPnl = profit;
	} else if (!closed && profit !== null) {
		netPnl = profit;
		unrealizedPnl = profit;
	}

	return {
		qty,
		entryQty: buys.qty,
		entryCost: settledCost,
		exitQty: sells.qty,
		exitProceeds: settledProceeds,
		entryPrice,
		mark,
		entryFee,
		exitFee,
		fees: null,
		exitVwap,
		exitRoom,
		grossPnl,
		netPnl,
		unrealizedPnl,
		returnPct,
		openedAt,
		closedAt,
		holdNs: holdNs as number | null,
		closed,
		exitPending:
			((row.exit_orders as Record<string, unknown>[])?.length ?? 0) > 0 &&
			!closed,
	};
};

export const onBook = (row: Record<string, unknown>, book: Book) =>
	row.virtual === (book === "virtual");

export const openOnBook = (rows: Record<string, unknown>[], book: Book) =>
	rows.filter((row) => onBook(row, book) && !positionReadout(row).closed);

export const closedOnBook = (rows: Record<string, unknown>[], book: Book) =>
	rows.filter((row) => onBook(row, book) && positionReadout(row).closed);

export const lotReturn = (row: Record<string, unknown>): number | null =>
	positionReadout(row).returnPct;

/* Closed lots own book performance; open lots stand in until one closes. */
export const bookReadout = (rows: Record<string, unknown>[], book: Book) => {
	const held = openOnBook(rows, book);
	const closed = closedOnBook(rows, book);
	const sample = closed.length > 0 ? closed : held;
	const returns = sample
		.map(lotReturn)
		.filter((value): value is number => value !== null);

	/*
	A frame where no lot prices yet (returns empty) must not blank the
	figures the previous frame showed; carry the last computed readout.
	*/
	const next =
		returns.length > 0
			? {
					winRate: returns.filter((value) => value > 0).length / returns.length,
					edge: returns.reduce((sum, value) => sum + value, 0) / returns.length,
					closed: closed.length,
				}
			: null;

	const carried = lastBookReadout.get(book);

	if (next === null) {
		return carried ?? { winRate: null, edge: null, closed: closed.length };
	}

	lastBookReadout.set(book, next);

	return next;
};

const lastBookReadout = new Map<
	Book,
	{ winRate: number; edge: number; closed: number }
>();
