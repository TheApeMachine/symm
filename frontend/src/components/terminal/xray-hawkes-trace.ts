export type HawkesTraceSample = {
	at: bigint;
	intensity: number;
	postArrival: number;
	baseline: number;
	decay: number;
};

export type HawkesTracePoint = { at: bigint; intensity: number };

/*
hawkesLevelAt evaluates λ along the decay published with one fitted event.
*/
export const hawkesLevelAt = (
	sample: HawkesTraceSample,
	at: bigint,
): number => {
	if (at <= sample.at) {
		return sample.intensity;
	}

	const seconds = Number(at - sample.at) / 1e9;
	const restBelowPeak = sample.baseline < sample.postArrival;

	if (restBelowPeak && sample.decay > 0) {
		return (
			sample.baseline +
			(sample.postArrival - sample.baseline) *
				Math.exp(-sample.decay * seconds)
		);
	}

	return sample.postArrival;
};

const segmentIntensity = (
	current: HawkesTraceSample,
	next: HawkesTraceSample | undefined,
	at: bigint,
	start: bigint,
	end: bigint,
	seconds: number,
	step: number,
	steps: number,
): number => {
	if (step === steps && next !== undefined) {
		return next.intensity;
	}

	const restBelowPeak = current.baseline < current.postArrival;
	const useExponential = restBelowPeak && current.decay > 0;

	if (!useExponential) {
		if (next !== undefined && end > start) {
			const progress = Number(at - start) / Number(end - start);
			return (
				current.postArrival +
				(next.intensity - current.postArrival) * progress
			);
		}
		return current.postArrival;
	}

	return (
		current.baseline +
		(current.postArrival - current.baseline) *
			Math.exp(-current.decay * seconds)
	);
};

/*
The viewport retains the observed event span (at least one fitted decay time).
Market time advances its right edge even when this symbol has no new arrivals.
Each segment uses the fit published with that event, never the newest fit
retroactively. Equal-time arrivals remain separate jumps.
*/
export const hawkesTrace = (
	samples: HawkesTraceSample[],
	horizontalPixels: number,
	marketAt: bigint = samples.at(-1)?.at ?? 0n,
	viewportSpan?: bigint,
) => {
	const first = samples[0];
	const last = samples.at(-1);
	const points: HawkesTracePoint[] = [];

	if (!first || !last) return { points, from: marketAt, through: marketAt };

	const through = marketAt > last.at ? marketAt : last.at;
	const eventSpan = last.at - first.at;
	const decaySpan = BigInt(Math.ceil(1e9 / last.decay));
	const span = viewportSpan ?? (eventSpan > decaySpan ? eventSpan : decaySpan);
	const from = through - span;

	for (let index = 0; index < samples.length; index++) {
		const current = samples[index] as HawkesTraceSample;
		const next = samples[index + 1];
		const end = next?.at ?? through;

		if (end < from || current.at > through) continue;

		if (current.at >= from) {
			points.push({ at: current.at, intensity: current.intensity });
			if (current.postArrival !== current.intensity) {
				points.push({ at: current.at, intensity: current.postArrival });
			}
		}

		const start = current.at > from ? current.at : from;
		const gap = end - start;

		if (gap <= 0n) continue;

		const steps = Math.max(
			1,
			Math.ceil((Number(gap) / Number(span)) * horizontalPixels),
		);

		for (let step = 0; step <= steps; step++) {
			const at = start + BigInt(Math.round((Number(gap) * step) / steps));
			const seconds = Number(at - current.at) / 1e9;
			points.push({
				at,
				intensity: segmentIntensity(
					current,
					next,
					at,
					start,
					end,
					seconds,
					step,
					steps,
				),
			});
		}
	}

	return { points, from, through };
};

const MIN_EVENT_GAP_NS = 100_000_000n;

/*
samplesForTrace drops aggregate ticks that share one frame timestamp. The
workspace publishes one clock per batch; stacking fifty revisions at the
same `at` draws a vertical comb, not Hawkes decay.
*/
export const samplesForTrace = (
	samples: HawkesTraceSample[],
): HawkesTraceSample[] => {
	if (samples.length === 0) return [];
	if (samples.length === 1) return samples;

	const sorted = [...samples].sort((left, right) =>
		left.at < right.at ? -1 : left.at > right.at ? 1 : 0,
	);
	const first = sorted[0];
	const last = sorted.at(-1);
	if (!first || !last) return sorted;
	const span = last.at - first.at;
	if (span < MIN_EVENT_GAP_NS) {
		return [last];
	}

	const out: HawkesTraceSample[] = [];
	for (const sample of sorted) {
		const tail = out.at(-1);
		if (tail && tail.at === sample.at) {
			out[out.length - 1] = sample;
		} else {
			out.push(sample);
		}
	}
	return out;
};

/*
orderTracePoints sorts λ(t) samples for drawing and merges duplicate times.
*/
export const orderTracePoints = (
	points: HawkesTracePoint[],
): HawkesTracePoint[] => {
	const sorted = [...points].sort((left, right) =>
		left.at < right.at ? -1 : left.at > right.at ? 1 : 0,
	);
	const out: HawkesTracePoint[] = [];
	for (const point of sorted) {
		const tail = out.at(-1);
		if (tail && tail.at === point.at) {
			out[out.length - 1] = point;
		} else {
			out.push(point);
		}
	}
	return out;
};
