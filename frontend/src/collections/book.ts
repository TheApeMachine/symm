import { createAtom } from "@tanstack/store";
import type { Decision } from "#/collections/envelope";

/*
bookAtom chooses which trades the dashboard reads. Live mode defaults to real;
virtual/paper remain available when market.model is paper.

The trie (model/paths) and sequencer stay shared across books — only the
desk ledgers (balances, lots, tiered decisions) switch with this atom.
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

/*
decisionBook maps a live decision onto a ledger tab. Explicit tier wins.
Pre-tier gate skips (oracle/resonance/story) are desk-agnostic — they follow
the selected book so a live REAL rail is not emptied by a virtual default.
*/
export const decisionBook = (decision: Decision): Book => {
	if (isBook(decision.tier)) {
		return decision.tier;
	}

	if (decision.virtual) {
		return "virtual";
	}

	return bookAtom.get();
};

export const decisionStoreKey = (decision: Decision): string =>
	`${decisionBook(decision)}:${decision.symbol}`;
