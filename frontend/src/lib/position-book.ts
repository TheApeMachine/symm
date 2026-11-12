import type { Position } from "#/collections/types";
import { isClosed } from "#/components/book-readout";

export const positionTier = (pos: { tier_key?: string }) =>
	typeof pos.tier_key === "string" && pos.tier_key !== ""
		? pos.tier_key
		: "paper";

const sameOpen = (left?: string | null, right?: string | null) => {
	if (!left || !right) return false;
	if (left === right) return true;

	const a = Date.parse(left);
	const b = Date.parse(right);

	return Number.isFinite(a) && a === b;
};

/*
dropClosedTwins removes an open lot when the same trip is already closed
under another id. History rebuilds the close with a new id and the chart
keeps the earlier open row otherwise.
*/
export const dropClosedTwins = (book: Record<string, Position>) => {
	const closed = Object.values(book).filter(
		(pos) => pos && isClosed(pos) && pos.symbol && pos.opened_at,
	);

	if (closed.length === 0) return book;

	let dropped = false;
	const next: Record<string, Position> = {};

	for (const [id, pos] of Object.entries(book)) {
		if (!pos) continue;

		const twin =
			!isClosed(pos) &&
			closed.some(
				(done) =>
					done.id !== pos.id &&
					done.symbol === pos.symbol &&
					positionTier(done) === positionTier(pos) &&
					sameOpen(done.opened_at, pos.opened_at),
			);

		if (twin) {
			dropped = true;
			continue;
		}

		next[id] = pos;
	}

	return dropped ? next : book;
};

/*
mergePositionBook applies one desk positions frame.

Mark ticks merge. A snapshot is the whole tier book: ids missing from it
leave, and other tiers stay.
*/
export const mergePositionBook = (
	prev: Record<string, Position> | null | undefined,
	incoming: Record<string, Position>,
	opts: { snapshot?: boolean; tier?: string },
): Record<string, Position> => {
	const prior = prev && typeof prev === "object" ? prev : {};

	if (!opts.snapshot) {
		return dropClosedTwins({ ...prior, ...incoming });
	}

	const tiers = new Set<string>();

	if (opts.tier) tiers.add(opts.tier);

	for (const pos of Object.values(incoming)) {
		if (pos && typeof pos === "object") tiers.add(positionTier(pos));
	}

	const kept: Record<string, Position> = {};

	for (const [id, pos] of Object.entries(prior)) {
		if (!pos || tiers.has(positionTier(pos))) continue;
		kept[id] = pos;
	}

	return dropClosedTwins({ ...kept, ...incoming });
};
