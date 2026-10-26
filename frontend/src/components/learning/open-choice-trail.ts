export type ChoiceSample = { t: number; pnl: number };

const trails = new Map<string, ChoiceSample[]>();
const MAX_SAMPLES = 180;

export const noteOpenChoice = (id: string, pnl: number, now = Date.now()) => {
	if (!Number.isFinite(pnl)) return;

	const prior = trails.get(id) ?? [];
	const last = prior[prior.length - 1];
	if (last && last.pnl === pnl) return;

	const next = [...prior, { t: now, pnl }];
	trails.set(
		id,
		next.length > MAX_SAMPLES ? next.slice(next.length - MAX_SAMPLES) : next,
	);
};

export const openChoiceTrail = (id: string): ChoiceSample[] =>
	trails.get(id) ?? [];

export const dropOpenChoices = (live: Set<string>) => {
	for (const id of trails.keys()) {
		if (!live.has(id)) trails.delete(id);
	}
};

export const resetOpenChoices = () => {
	trails.clear();
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
