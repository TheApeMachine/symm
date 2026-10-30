export type ChoiceSample = { t: number; pnl: number };

/*
Clock-window helpers for a fixed lookback. The open-choices chart does not
use them: it draws the same sample trail as the position-card sparkline.
*/
export const CHOICE_WINDOW_MS = 1 * 60 * 1000;

export const choiceWindow = (now: number, windowMs = CHOICE_WINDOW_MS) => ({
	from: now - windowMs,
	through: now,
	span: windowMs,
});

/*
samplesInWindow holds the last return into the left edge, then keeps every
sample inside the window. The held point uses `from` as its time. Keeping
the older timestamp draws a ramp across the minute for a change that
happened at one instant, and that ramp slides as the window moves.
*/
export const samplesInWindow = (
	samples: ChoiceSample[],
	from: number,
): ChoiceSample[] => {
	if (samples.length === 0) return [];

	let firstInside = samples.length;
	for (let i = 0; i < samples.length; i++) {
		if (samples[i].t >= from) {
			firstInside = i;
			break;
		}
	}

	if (firstInside === samples.length) {
		const last = samples[samples.length - 1];
		return [{ t: from, pnl: last.pnl }];
	}

	if (firstInside === 0) return samples;

	const held = samples[firstInside - 1];
	return [{ t: from, pnl: held.pnl }, ...samples.slice(firstInside)];
};

/*
stepPath holds the previous return until the sample time, then jumps.
Close-now return is constant between the samples we kept.
*/
export const stepPath = (
	samples: ChoiceSample[],
	xOf: (t: number) => number,
	yOf: (pnl: number) => number,
) => {
	if (samples.length === 0) return "";

	const x = (t: number) => xOf(t).toFixed(1);
	const y = (pnl: number) => yOf(pnl).toFixed(1);
	let path = `M${x(samples[0].t)},${y(samples[0].pnl)}`;

	for (let i = 1; i < samples.length; i++) {
		path += ` L${x(samples[i].t)},${y(samples[i - 1].pnl)}`;
		path += ` L${x(samples[i].t)},${y(samples[i].pnl)}`;
	}

	return path;
};

const trails = new Map<string, ChoiceSample[]>();
const sampleAt = new Map<string, number>();

/** Same cadence and length as the per-lot P&L trail on position cards. */
export const CHOICE_SAMPLE_MS = 250;
export const CHOICE_TRAIL_LENGTH = 90;

export const noteOpenChoice = (
	id: string,
	pnl: number,
	now = Date.now(),
): boolean => {
	if (!Number.isFinite(pnl)) return false;

	const prior = trails.get(id) ?? [];
	const last = prior[prior.length - 1];
	const lastAt = sampleAt.get(id) ?? 0;
	const changed = !last || last.pnl !== pnl;
	if (!changed && now - lastAt < CHOICE_SAMPLE_MS) return false;

	const next = [...prior, { t: now, pnl }];
	trails.set(
		id,
		next.length > CHOICE_TRAIL_LENGTH
			? next.slice(next.length - CHOICE_TRAIL_LENGTH)
			: next,
	);
	sampleAt.set(id, now);
	return true;
};

export const openChoiceTrail = (id: string): ChoiceSample[] =>
	trails.get(id) ?? [];

export const dropOpenChoices = (live: Set<string>): boolean => {
	let dropped = false;
	for (const id of trails.keys()) {
		if (!live.has(id)) {
			trails.delete(id);
			sampleAt.delete(id);
			dropped = true;
		}
	}
	return dropped;
};

export const resetOpenChoices = () => {
	trails.clear();
	sampleAt.clear();
};

export type OpenChoiceLot = { id: string; pnl: number | null };

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

export const extendToNow = (
	samples: ChoiceSample[],
	now: number,
): ChoiceSample[] => {
	if (samples.length === 0) return [];
	const last = samples[samples.length - 1];
	if (now <= last.t) return samples;
	return [...samples, { t: now, pnl: last.pnl }];
};

/*
meanPath is the equal-weight close-now return of the lots that have a
sample by each instant. A lot joins the average once it has been seen.
*/
export const meanPath = (
	series: ChoiceSample[][],
	now: number,
): ChoiceSample[] => {
	const extended = series
		.map((samples) => extendToNow(samples, now))
		.filter((samples) => samples.length > 0);
	if (extended.length === 0) return [];

	const times = [
		...new Set(
			extended.flatMap((samples) => samples.map((sample) => sample.t)),
		),
	].sort((a, b) => a - b);
	const cursor = extended.map(() => 0);
	const latest = extended.map(() => null as number | null);
	const out: ChoiceSample[] = [];

	for (const t of times) {
		for (let i = 0; i < extended.length; i++) {
			const samples = extended[i];
			while (cursor[i] < samples.length && samples[cursor[i]].t <= t) {
				latest[i] = samples[cursor[i]].pnl;
				cursor[i]++;
			}
		}

		const known = latest.filter((value): value is number => value !== null);
		if (known.length === 0) continue;
		out.push({
			t,
			pnl: known.reduce((sum, value) => sum + value, 0) / known.length,
		});
	}

	return out;
};

/*
linePath joins samples in order, the same way the position-card Trail does.
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
