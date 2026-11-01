import { createAtom } from "@tanstack/store";
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

const emptyBook = (): BookConfidence => ({
	active: false,
	armed: false,
	halted: false,
	closed: 0,
	wins: 0,
	win_rate: null,
	edge: null,
});

export const confidenceAtom = createAtom<ConfidenceFrame>({
	virtual: { ...emptyBook(), active: true },
	paper: { ...emptyBook(), active: true },
	real: emptyBook(),
	eligible_resistance: 0.2,
	real_ceiling: false,
});

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

const asRate = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const asCount = (value: unknown): number =>
	typeof value === "number" && Number.isFinite(value) ? value : 0;

const asBook = (value: unknown, fallbackActive = false): BookConfidence => {
	if (!value || typeof value !== "object") {
		return { ...emptyBook(), active: fallbackActive };
	}

	const row = value as Record<string, unknown>;

	return {
		active: row.active === true || fallbackActive,
		armed: row.armed === true,
		halted: row.halted === true,
		closed: asCount(row.closed),
		wins: asCount(row.wins),
		win_rate: asRate(row.win_rate),
		edge: asRate(row.edge),
	};
};

export const ingestConfidence = (raw: unknown) => {
	if (!raw || typeof raw !== "object") return;

	const row = raw as Record<string, unknown>;

	confidenceAtom.set({
		virtual: asBook(row.virtual, true),
		paper: asBook(row.paper, true),
		real: asBook(row.real),
		eligible_resistance:
			typeof row.eligible_resistance === "number"
				? row.eligible_resistance
				: 0.2,
		real_ceiling: row.real_ceiling === true,
	});
};
