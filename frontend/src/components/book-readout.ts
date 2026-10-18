import type { Book } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";

/*
lotReturn is the fee-net fraction when the desk sent one, otherwise the
distance from entry to the mark.
*/
export const lotReturn = (row: DeskPosition): number | null => {
	if (row.returnPct !== null && Number.isFinite(row.returnPct)) {
		return row.returnPct;
	}

	const entry = Number(row.entryPrice);
	const mark = Number(row.mark);

	if (!Number.isFinite(entry) || entry === 0 || !Number.isFinite(mark)) {
		return null;
	}

	return (mark - entry) / entry;
};

const onBook = (row: DeskPosition, book: Book) =>
	row.virtual === (book === "virtual");

/*
bookReadout is the top bar's win rate and mean edge for one book. Closed
round trips own both. While none have closed, the open lots do. Unrealized
and equity are the trade balance, not this readout.
*/
export const bookReadout = (rows: DeskPosition[], book: Book) => {
	const held = rows.filter((row) => onBook(row, book) && row.closedAt === null);
	const closed = rows.filter((row) => onBook(row, book) && row.closedAt !== null);
	const sample = closed.length > 0 ? closed : held;
	const returns = sample
		.map(lotReturn)
		.filter((value): value is number => value !== null);

	return {
		winRate: returns.length > 0 ? returns.filter((value) => value > 0).length / returns.length : null,
		edge: returns.length > 0 ? returns.reduce((sum, value) => sum + value, 0) / returns.length : null,
		closed: closed.length,
	};
};
