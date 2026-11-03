import { createAtom } from "@tanstack/store";
import type { Decision } from "#/collections/envelope";

/*
bookAtom chooses which trades the dashboard reads. Virtual explores paths,
paper validates them, real spends the live wallet once confidence is armed.

The trie (model/paths) and sequencer stay shared across books — only the
desk ledgers (balances, lots, tiered decisions) switch with this atom.
*/
export type Book = "paper" | "virtual" | "real";

export const bookAtom = createAtom<Book>("virtual");

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
decisionBook maps a live decision onto a ledger tab. Explicit tier wins;
legacy `virtual` and pre-tier model skips (resonance/path gates) surface
under virtual — paper/real only show outcomes stamped for that desk.
*/
export const decisionBook = (decision: Decision): Book => {
	if (isBook(decision.tier)) {
		return decision.tier;
	}

	if (decision.virtual) {
		return "virtual";
	}

	return "virtual";
};

export const decisionStoreKey = (decision: Decision): string =>
	`${decisionBook(decision)}:${decision.symbol}`;
