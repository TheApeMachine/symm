import { closeNowReturnPct } from "#/components/book-readout";

export type ChoiceSample = { t: number; pnl: number };

const trails = new Map<string, ChoiceSample[]>();

/** Ring buffer length shared by ui/Trail on cards and the open-choices chart. */
export const CHOICE_TRAIL_LENGTH = 90;

/*
noteOpenChoice appends one close-now sample. Call it when the positions store
delivers a new mark, not on a wall-clock timer.
*/
export const noteOpenChoice = (
	id: string,
	pnl: number,
	now = Date.now(),
): boolean => {
	if (!Number.isFinite(pnl)) return false;

	const prior = trails.get(id) ?? [];
	const next = [...prior, { t: now, pnl }];
	trails.set(
		id,
		next.length > CHOICE_TRAIL_LENGTH
			? next.slice(next.length - CHOICE_TRAIL_LENGTH)
			: next,
	);

	return true;
};

export const openChoiceTrail = (id: string): ChoiceSample[] =>
	trails.get(id) ?? [];

/** Percent series for ui/Trail and the open-choices chart (same buffer). */
export const openChoiceValues = (id: string): number[] =>
	openChoiceTrail(id).map((sample) => sample.pnl);

export const dropOpenChoices = (live: Set<string>): boolean => {
	let dropped = false;
	for (const id of trails.keys()) {
		if (!live.has(id)) {
			trails.delete(id);
			dropped = true;
		}
	}
	return dropped;
};

export const resetOpenChoices = () => {
	trails.clear();
};

export type OpenChoiceLot = { id: string; pnl: number | null };

export const openChoiceLotsFromRows = (
	rows: Record<string, unknown>[],
): OpenChoiceLot[] =>
	rows.map((row) => ({
		id: String(row.id ?? row.symbol),
		pnl: closeNowReturnPct(row),
	}));

/** Record close-now samples for the open lot set; returns true if trails changed. */
export const syncOpenChoiceLots = (rows: OpenChoiceLot[]): boolean => {
	const live = new Set<string>();
	let dirty = false;
	for (const { id, pnl } of rows) {
		live.add(id);
		if (pnl !== null && noteOpenChoice(id, pnl)) dirty = true;
	}
	if (dropOpenChoices(live)) dirty = true;
	return dirty;
};

/*
linePath joins samples in order, the same way ui/Trail does.
Index 0 is the oldest kept sample and the last index is now.
*/
export const linePath = (
	points: { x: number; y: number }[],
): string =>
	points
		.map(
			(point, i) =>
				`${i === 0 ? "M" : "L"}${point.x.toFixed(1)},${point.y.toFixed(1)}`,
		)
		.join(" ");

/*
indexMean right-aligns each lot on a shared sample index so now is the last
slot, then takes the equal-weight return of the lots that have a sample there.
*/
export const indexMean = (series: ChoiceSample[][]): number[] => {
	const maxLen = series.reduce((n, samples) => Math.max(n, samples.length), 0);
	const mean: number[] = [];

	for (let i = 0; i < maxLen; i++) {
		let sum = 0;
		let count = 0;
		for (const samples of series) {
			const idx = i - (maxLen - samples.length);
			if (idx < 0) continue;
			const pnl = samples[idx]?.pnl;
			if (pnl === undefined || !Number.isFinite(pnl)) continue;
			sum += pnl;
			count++;
		}
		mean.push(count > 0 ? sum / count : Number.NaN);
	}

	return mean;
};
