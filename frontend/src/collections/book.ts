import { createAtom } from "@tanstack/store";
import type { Decision } from "#/collections/envelope";

/*
bookAtom selects which tier tab the dashboard shows (balances, positions, decisions).
Defaults to real so live money is what you see on load.

The trie (model/paths) and sequencer stay shared — only desk ledgers switch with the tab.
*/
export type Book = "paper" | "virtual" | "real";

export const bookAtom = createAtom<Book>("real");

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

export const isBook = (value: unknown): value is Book =>
	value === "virtual" || value === "paper" || value === "real";

export const decisionStoreKey = (decision: Decision): string =>
	isBook(decision.tier)
		? `${decision.tier}:${decision.symbol}`
		: decision.symbol;