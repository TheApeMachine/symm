export type HawkesTraceSample = {
	at: bigint;
	intensity: number;
	postArrival: number;
	baseline: number;
	decay: number;
};

export type HawkesTracePoint = { at: bigint; intensity: number };

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
) => {
	const first = samples[0];
	const last = samples.at(-1);
	const points: HawkesTracePoint[] = [];

	if (!first || !last) return { points, from: marketAt, through: marketAt };

	const through = marketAt > last.at ? marketAt : last.at;
	const eventSpan = last.at - first.at;
	const decaySpan = BigInt(Math.ceil(1e9 / last.decay));
	const span = eventSpan > decaySpan ? eventSpan : decaySpan;
	const from = through - span;

	for (let index = 0; index < samples.length; index++) {
		const current = samples[index] as HawkesTraceSample;
		const next = samples[index + 1];
		const end = next?.at ?? through;

		if (end < from || current.at > through) continue;

		if (current.at >= from) {
			points.push({ at: current.at, intensity: current.intensity });
			points.push({ at: current.at, intensity: current.postArrival });
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
				intensity:
					current.baseline +
					(current.postArrival - current.baseline) *
						Math.exp(-current.decay * seconds),
			});
		}
	}

	return { points, from, through };
};
