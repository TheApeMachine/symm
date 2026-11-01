import { createAtom } from "@tanstack/store";

/*
bookAtom chooses which trades the dashboard reads. Virtual explores paths,
paper validates them, real spends the live wallet once confidence is armed.
*/
export type Book = "paper" | "virtual" | "real";

export const bookAtom = createAtom<Book>("paper");

export const triePrefix = (_book: Book = bookAtom.get()) => "model/paths";

export const bookLabel = (book: Book): string => {
	switch (book) {
		case "virtual":
			return "Virtual";
		case "real":
			return "Real";
		default:
			return "Paper";
	}
};
