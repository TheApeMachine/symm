import type { Book } from "#/collections/book";
import type { GaugeMark } from "#/components/ui/gauge";

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

	const cost = decimal(row.cost);
	const profit = decimal(row.profit);
	if (cost !== null && profit !== null && cost > 0) {
		return profit / cost;
	}

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
	const cost = decimal(row.cost);
	const profit = decimal(row.profit);

	let pnl = frac;
	if (pnl === null && profit !== null && cost !== null && cost > 0) {
		pnl = profit / cost;
	}

	if (pnl === null && pnlPct === null) return null;

	return {
		pnl,
		pnl_pct: pnlPct ?? (pnl === null ? null : pnl * 100),
		hold_ns: decimal(row.hold_ns),
		triggers: outcomeTriggers(row.outcome_triggers),
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

/*
Open PnL and exit_vwap come from broker/price book walks; exit_price is the settled fill when closed.
Fills only help detect venue round trips when closed_at is absent.
*/
const positionCostQuote = (row: Record<string, unknown>): number | null => {
	const pnl = decimal(row.pnl);
	const pnlPct = decimal(row.pnl_pct);
	if (pnl !== null && pnlPct !== null && pnlPct !== 0) {
		return pnl / (pnlPct / 100);
	}

	const qty = decimal(row.qty);
	const entry = decimal(row.entry_price);
	if (qty !== null && entry !== null) {
		return qty * entry;
	}

	return null;
};

/*
Maps close-now return to the P&L gauge fill position (0 = max loss, 0.5 = flat, 1 = max gain).
*/
export const pnlGaugeValue = (returnPct: number | undefined): number | undefined => {
	if (returnPct === undefined) return undefined;
	return 0.5 + Math.max(-0.5, Math.min(0.5, returnPct * 5));
};

/*
Book explore exits: profit at zero close-now P&L; stop at twice the opening underwater gap.
*/
export const exploreExitGaugeMarks = (
	row: Record<string, unknown>,
): GaugeMark[] => {
	const marks: GaugeMark[] = [
		{
			at: 0.5,
			color: "var(--up)",
			title: "Profit exit — close-now P&L above zero",
		},
	];

	if (row.virtual !== true) {
		return marks;
	}

	const openPnl = decimal(row.open_pnl);
	if (openPnl === null || openPnl >= 0) {
		return marks;
	}

	const cost = positionCostQuote(row);
	if (cost === null || cost <= 0) {
		return marks;
	}

	const stopReturn = (2 * openPnl) / cost;
	const stopAt = pnlGaugeValue(stopReturn);
	if (stopAt === undefined) {
		return marks;
	}

	marks.push({
		at: stopAt,
		color: "var(--down)",
		title: `Stop exit — symmetric book hurdle (${(stopReturn * 100).toFixed(2)}% close-now)`,
	});

	return marks;
};

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
				decimal(row.pnl_pct) !== null ||
				(decimal(row.cost) !== null && decimal(row.profit) !== null)
			: fullySold;

	const closedAt = dateText(row.closed_at) ?? (fullySold ? sells.lastAt : null);
	const holdNs =
		row.hold_ns ??
		(closedAt !== null && openedAt !== null
			? (Date.parse(closedAt) - Date.parse(openedAt)) * 1e6
			: null);

	const returnPct = returnFraction(row);
	const returnPctDisplay = decimal(row.pnl_pct);

	return {
		qty,
		entryQty: buys.qty,
		entryCost: buys.cost,
		exitQty: sells.qty,
		entryPrice,
		mark,
		entryFee,
		exitFee,
		fees: null,
		exitVwap,
		exitRoom,
		returnPct,
		returnPctDisplay,
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

const countsTowardBookPerformance = (
	row: Record<string, unknown>,
	book: Book,
): boolean => book !== "virtual" || row.path_found === true;

export const lotReturn = (row: Record<string, unknown>): number | null =>
	positionReadout(row).returnPct;

/* Closed lots own book performance; open lots stand in until one closes. */
export const bookReadout = (rows: Record<string, unknown>[], book: Book) => {
	const held = openOnBook(rows, book).filter((row) =>
		countsTowardBookPerformance(row, book),
	);
	const closed = closedOnBook(rows, book).filter((row) =>
		countsTowardBookPerformance(row, book),
	);
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
