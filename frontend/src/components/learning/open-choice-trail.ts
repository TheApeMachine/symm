import { closeNowReturnPct } from "#/components/book-readout";

export type ChoiceSample = { t: number; pnl: number; stamp: string };

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
	stamp: string,
	now = Date.now(),
): boolean => {
	if (!Number.isFinite(pnl) || stamp === "") return false;

	const prior = trails.get(id) ?? [];
	const last = prior[prior.length - 1];
	if (last && last.stamp === stamp) return false;

	const next = [...prior, { t: now, pnl, stamp }];
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

const STROKES = [
	"#7eb6ff",
	"#f0b429",
	"#5eead4",
	"#c4b5fd",
	"#e07a7a",
	"#86efac",
	"#f9a8d4",
];

const strokeFor = (id: string) => {
	let hash = 0;
	for (let i = 0; i < id.length; i++) {
		hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
	}
	return STROKES[hash % STROKES.length];
};

export type OpenChoiceSeries = {
	id: string;
	name: string;
	values: number[];
	stroke: string;
};

/** Close-now series for the open-choices chart (stable id order, no SciChart). */
export const openChoiceSeries = (
	lots: Record<string, unknown>[],
): OpenChoiceSeries[] => {
	const mapped = lots.map((row) => {
		const id = String(row.id ?? row.symbol);
		return {
			id,
			name: String(row.symbol ?? id),
			values: openChoiceValues(id),
			stroke: strokeFor(id),
		};
	});

	mapped.sort((a, b) => a.id.localeCompare(b.id));
	return mapped;
};

/** Legend order: hottest |P&L| first. */
export const openChoiceLegend = (
	series: OpenChoiceSeries[],
): OpenChoiceSeries[] =>
	[...series].sort((a, b) => {
		const aLast = a.values.length > 0 ? a.values[a.values.length - 1] : 0;
		const bLast = b.values.length > 0 ? b.values[b.values.length - 1] : 0;
		return Math.abs(bLast) - Math.abs(aLast);
	});

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

export type OpenChoiceLot = { id: string; pnl: number | null; stamp: string };

export const openChoiceMarkStamp = (row: Record<string, unknown>): string =>
	[
		row.pnl_pct,
		row.mark,
		row.peak_pnl_pct,
		row.exit_vwap,
		row.bars_held,
	].join("|");

export const openChoiceLotsFromRows = (
	rows: Record<string, unknown>[],
): OpenChoiceLot[] =>
	rows.map((row) => ({
		id: String(row.id ?? row.symbol),
		pnl: closeNowReturnPct(row),
		stamp: openChoiceMarkStamp(row),
	}));

/** Record close-now samples for the open lot set; returns true if trails changed. */
export const syncOpenChoiceLots = (rows: OpenChoiceLot[]): boolean => {
	const live = new Set<string>();
	let dirty = false;
	for (const { id, pnl, stamp } of rows) {
		live.add(id);
		if (pnl !== null && noteOpenChoice(id, pnl, stamp)) dirty = true;
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
