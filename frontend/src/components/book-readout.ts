import type { Book } from "#/collections/book";
import type { Position } from "#/collections/types";

const decimalPattern = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;

/** JSON decimals arrive as string | number. */
export const decimal = (value: unknown): number | null => {
	if (typeof value === "number") return Number.isFinite(value) ? value : null;

	if (typeof value !== "string") return null;

	const text = value.trim();
	if (!decimalPattern.test(text)) return null;

	const n = Number(text);
	return Number.isFinite(n) ? n : null;
};

export const isClosed = (pos: Position) =>
	pos.closed_at != null && pos.closed_at !== "";

export const pnlPct = (pos: Position) => decimal(pos.pnl_pct);

export const pnlFrac = (pos: Position) => {
	const pct = pnlPct(pos);
	return pct === null ? null : pct / 100;
};

export const holdNs = (pos: Position) => {
	if (!isClosed(pos) || !pos.opened_at || !pos.closed_at) return null;

	const opened = Date.parse(pos.opened_at);
	const closed = Date.parse(pos.closed_at);
	if (!Number.isFinite(opened) || !Number.isFinite(closed)) return null;

	return (closed - opened) * 1e6;
};

/*
Match a desk row to the selected ledger. Lots without a tier_key default to
paper so virtual/real tabs stay empty until labeled.
*/
export const onTier = (
	pos: Position | { tier_key?: string },
	book: Book,
) => {
	if (typeof pos.tier_key === "string" && pos.tier_key !== "") {
		return pos.tier_key === book;
	}

	return book === "paper";
};

export const openOnTier = (rows: Position[], book: Book) =>
	rows.filter((row) => onTier(row, book) && !isClosed(row));

export const closedOnTier = (rows: Position[], book: Book) =>
	rows.filter((row) => onTier(row, book) && isClosed(row));

export const countsTowardBookPerformance = (row: Position, book: Book) =>
	onTier(row, book);
