import type { Book } from "#/collections/book";

const decimalPattern = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;

const decimal = (value: unknown): number | null => {
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

/* The desk's settled outcome, when it has priced the close. */
export const settledOutcome = (row: Record<string, unknown>) => {
	if (row.cost === undefined || row.proceeds === undefined) return null;

	const cost = decimal(row.cost);
	const proceeds = decimal(row.proceeds);

	if (cost === null || proceeds === null) return null;

	return {
		cost,
		proceeds,
		fees:
			decimal(row.entry_fee ?? null) === null &&
			decimal(row.exit_fee ?? null) === null
				? null
				: (decimal(row.entry_fee ?? 0) ?? 0) +
					(decimal(row.exit_fee ?? 0) ?? 0),
		realized: decimal(row.profit) ?? proceeds - cost,
		net_return: row.net_return ?? row.return ?? null,
		hold_ns: row.hold_ns ?? null,
		triggers: row.outcome_triggers ?? null,
	};
};

type ExecutionTotals = {
	qty: number | null;
	cost: number | null;
	fee: number | null;
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

		const fillQty = decimal(fill.last_qty);
		if (fillQty === null) continue;

		const fillCost = decimal(fill.cost);
		const fillFee = decimal(fill.fee_usd_equiv);
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

		firstAt ??= fillAt;
		lastAt = fillAt ?? lastAt;
	}

	return {
		qty: sawFill ? qty : null,
		cost: sawFill && costKnown ? cost : null,
		fee: sawFill && feeKnown ? fee : null,
		firstAt,
		lastAt,
	};
};

const orderQuantity = (orders: Record<string, unknown>[]) => {
	if (!orders || orders.length === 0) return null;

	let total = 0;
	for (const order of orders) {
		const quantity = decimal(order.volume);
		if (quantity === null) return null;
		total += quantity;
	}

	return total;
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

/* Position metrics come from order and execution facts; mark is a live price input. */
export const positionReadout = (
	row: Record<string, unknown>,
): PositionReadout => {
	const buys = executionTotals(row, "buy");
	const sells = executionTotals(row, "sell");
	const entryOrderQty = orderQuantity(
		row.entry_orders as Record<string, unknown>[],
	);
	const qty = buys.qty === null ? entryOrderQty : buys.qty - (sells.qty ?? 0);
	const entryPrice =
		buys.qty !== null && buys.cost !== null && buys.qty !== 0
			? buys.cost / buys.qty
			: null;
	const mark = decimal(row.mark);
	const exitVwap =
		sells.qty !== null && sells.cost !== null && sells.qty !== 0
			? sells.cost / sells.qty
			: null;
	const openedAt = dateText(row.opened_at) ?? buys.firstAt;
	const fullySold =
		buys.qty !== null && sells.qty !== null && sells.qty >= buys.qty;

	// The desk stamps closed_at when it moves a lot to its closed cache.
	// A closed virtual settles emulated (no venue sell fill), so it only
	// counts once the settlement outcome (cost + proceeds) has landed.
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
	const entryFee = decimal(row.entry_fee);
	const exitFee = decimal(row.exit_fee);

	/*
	Fee fields are percent rates once the desk is marking (Update stamps
	the tier rate on every tick). A dollar fee only ever arrives on a
	venue-echoed execution against a notional; without one, rates are the
	fraction of cost, which the percent readout needs re-based.
	*/
	const feeFraction = (rate: number | null): number | null =>
		rate === null ? null : rate / 100;

	const entryFeeFraction = feeFraction(entryFee);
	const exitFeeFraction = feeFraction(exitFee);
	const fees =
		entryFeeFraction !== null &&
		(sells.qty === null || exitFeeFraction !== null)
			? entryFeeFraction + (exitFeeFraction ?? 0)
			: null;
	const exitRoom = decimal(row.exit_room);
	const bookReturn = decimal(row.book_return);
	let grossPnl: number | null = null;
	let netPnl: number | null = null;
	let unrealizedPnl: number | null = null;
	let returnPct: number | null = null;

	// Settled round trips (the desk priced the close) are authoritative.
	const settledCost = decimal(row.cost);
	const settledProceeds = decimal(row.proceeds);
	const settledReturn = row.net_return ?? row.return ?? null;

	if (closed && settledCost !== null && settledProceeds !== null) {
		grossPnl = settledProceeds - settledCost;
		netPnl = decimal(row.profit) ?? grossPnl;
		returnPct =
			settledReturn !== null
				? (settledReturn as number) / 100
				: settledCost !== 0
					? netPnl / settledCost
					: null;
	} else if (buys.qty !== null && buys.cost !== null && buys.qty !== 0) {
		const soldQty = sells.qty ?? 0;
		const soldBasis = buys.cost * (soldQty / buys.qty);
		const realized =
			sells.qty === null
				? closed
					? null
					: 0
				: sells.cost === null
					? null
					: sells.cost - soldBasis;
		const remainingQty = buys.qty - soldQty;
		const remainingBasis = buys.cost * (remainingQty / buys.qty);
		const unrealized = closed
			? 0
			: mark === null || qty === null
				? null
				: mark * qty - remainingBasis;

		if (
			!closed &&
			unrealized !== null &&
			qty !== null &&
			entryFeeFraction !== null
		) {
			unrealizedPnl =
				unrealized - entryFeeFraction * buys.cost * (qty / buys.qty);
		}

		if (realized !== null && unrealized !== null) {
			grossPnl = realized + unrealized;
		}

		if (grossPnl !== null && fees !== null) {
			netPnl = grossPnl - fees;
			returnPct = netPnl / buys.cost;
		}
	}

	if (!closed && bookReturn !== null && entryPrice !== null && entryPrice > 0) {
		returnPct = bookReturn / 100;
	}

	return {
		qty,
		entryQty: buys.qty,
		entryCost: settledCost ?? buys.cost,
		exitQty: sells.qty,
		exitProceeds: settledProceeds ?? sells.cost,
		entryPrice,
		mark,
		entryFee,
		exitFee,
		fees,
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
