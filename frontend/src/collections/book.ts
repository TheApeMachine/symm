import { createAtom } from "@tanstack/store";

/*
bookAtom chooses which trades the dashboard reads. Paper is the desk.
Virtual is the learning book. The same switch picks the radix trie:
paper reads model/paths, virtual reads model/paths.
*/
export type Book = "paper" | "virtual";

export const bookAtom = createAtom<Book>("paper");

export const triePrefix = (book: Book = bookAtom.get()) => "model/paths";
