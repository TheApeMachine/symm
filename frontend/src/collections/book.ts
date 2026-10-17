import { createAtom } from "@tanstack/store";

/*
bookAtom chooses which trades the dashboard reads. Paper is the desk.
Virtual is the $200-stake experiments on finished rules.
*/
export type Book = "paper" | "virtual";

export const bookAtom = createAtom<Book>("paper");
