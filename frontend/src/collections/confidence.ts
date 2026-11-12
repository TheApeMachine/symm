import { atoms } from "#/collections/app";
import type { Book } from "#/collections/book";

export type BookConfidence = {
	active?: boolean;
	armed?: boolean;
	halted?: boolean;
	closed: number;
	wins: number;
	win_rate: number | null;
	edge: number | null;
};

export type ConfidenceFrame = {
	virtual: BookConfidence;
	paper: BookConfidence;
	real: BookConfidence;
	eligible_resistance: number;
	real_ceiling: boolean;
};

/*
confidence lives on atoms.confidence — the hub key matches the atom key.
*/
export const confidenceAtom = atoms.confidence;

export const confidenceFor = (
	frame: ConfidenceFrame,
	book: Book,
): BookConfidence => {
	switch (book) {
		case "virtual":
			return frame.virtual;
		case "real":
			return frame.real;
		default:
			return frame.paper;
	}
};
