import type { Book } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";

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

const epochText = (value: unknown): string | null => {
	const epoch = decimal(value);
	if (epoch === null || epoch === 0) return null;

	try {
		return new Date(epoch / 1e6).toISOString();
	} catch {
		return null;
	}
};

type ExecutionTotals = {
	qty: number | null;
	cost: number | null;
	fee: number | null;
	firstAt: string | null;
	lastAt: string | null;
};

const executionTotals = (
	row: DeskPosition,
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

	for (const fill of row.fills ?? []) {
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

const orderQuantity = (orders: DeskPosition["entry_orders"]) => {
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
export const positionReadout = (row: DeskPosition): PositionReadout => {
	const buys = executionTotals(row, "buy");
	const sells = executionTotals(row, "sell");
	const entryOrderQty = orderQuantity(row.entry_orders);
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
	const openedAt = epochText(row.entry_epoch) ?? buys.firstAt;
	const exitEpoch = epochText(row.exit_epoch);
	const fullySold =
		buys.qty !== null && sells.qty !== null && sells.qty >= buys.qty;
	const closed = exitEpoch !== null || fullySold;
	const closedAt = exitEpoch ?? (fullySold ? sells.lastAt : null);
	const holdNs =
		closedAt !== null && openedAt !== null
			? (Date.parse(closedAt) - Date.parse(openedAt)) * 1e6
			: null;
	const entryFee = buys.fee;
	const exitFee = sells.fee;
	const fees =
		entryFee !== null && (sells.qty === null || exitFee !== null)
			? entryFee + (exitFee ?? 0)
			: null;
	const exitRoom = decimal(row.exit_room);
	let grossPnl: number | null = null;
	let netPnl: number | null = null;
	let unrealizedPnl: number | null = null;
	let returnPct: number | null = null;

	if (buys.qty !== null && buys.cost !== null && buys.qty !== 0) {
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
			entryFee !== null
		) {
			unrealizedPnl = unrealized - entryFee * (qty / buys.qty);
		}

		if (realized !== null && unrealized !== null) {
			grossPnl = realized + unrealized;
		}

		if (grossPnl !== null && fees !== null) {
			netPnl = grossPnl - fees;
			returnPct = netPnl / buys.cost;
		}
	}

	return {
		qty,
	entryQty: buys.qty,
	entryCost: buys.cost,
		exitQty: sells.qty,
		exitProceeds: sells.cost,
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
		holdNs,
		closed,
		exitPending: (row.exit_orders?.length ?? 0) > 0 && !closed,
	};
};

export const onBook = (row: DeskPosition, book: Book) =>
	row.virtual === (book === "virtual");

export const openOnBook = (rows: DeskPosition[], book: Book) =>
	rows.filter((row) => onBook(row, book) && !positionReadout(row).closed);

export const closedOnBook = (rows: DeskPosition[], book: Book) =>
	rows.filter((row) => onBook(row, book) && positionReadout(row).closed);

export const lotReturn = (row: DeskPosition): number | null =>
	positionReadout(row).returnPct;

/* Closed lots own book performance; open lots stand in until one closes. */
export const bookReadout = (rows: DeskPosition[], book: Book) => {
	const held = openOnBook(rows, book);
	const closed = closedOnBook(rows, book);
	const sample = closed.length > 0 ? closed : held;
	const returns = sample
		.map(lotReturn)
		.filter((value): value is number => value !== null);

	return {
		winRate:
			returns.length > 0
				? returns.filter((value) => value > 0).length / returns.length
				: null,
		edge:
			returns.length > 0
				? returns.reduce((sum, value) => sum + value, 0) / returns.length
				: null,
		closed: closed.length,
	};
};