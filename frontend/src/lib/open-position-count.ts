import { atoms, signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import { openPositions, positionRows } from "#/components/learning/open-positions";

/*
syncOpenPositionCount keeps atoms.positionCount aligned with the Positions
readout in the top bar: open positions on the selected book.
*/
export const syncOpenPositionCount = (book: Book = bookAtom.get()) => {
	const count = openPositions(positionRows(signals.positions.state), book).length;
	atoms.positionCount.set(count);
};
