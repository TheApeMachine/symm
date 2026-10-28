import { createAtom } from "@tanstack/store";

/*
bookAtom chooses which trades the dashboard reads. Paper is the desk.
Virtual is the learning book. The same switch picks the radix trie:
paper reads model/policy, virtual reads model/explore.
*/
export type Book = "paper" | "virtual";

export const bookAtom = createAtom<Book>("paper");

export const triePrefix = (book: Book = bookAtom.get()) =>
	book === "paper" ? "model/policy" : "model/explore";
