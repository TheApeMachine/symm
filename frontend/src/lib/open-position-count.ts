import { atoms, signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import { openLots, positionRows } from "#/components/learning/open-positions";

/*
syncOpenPositionCount keeps atoms.positionCount aligned with the Positions
readout in the top bar: open lots on the selected book.
*/
export const syncOpenPositionCount = (book: Book = bookAtom.get()) => {
	const count = openLots(positionRows(signals.positions.state), book).length;
	atoms.positionCount.set(count);
};
