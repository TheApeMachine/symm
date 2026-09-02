export type HawkesTraceSample = {
	at: bigint;
	intensity: number;
	postArrival: number;
	baseline: number;
	decay: number;
};

export type HawkesTracePoint = {
	at: bigint;
	intensity: number;
};

/*
hawkesTrace expands each observed arrival into its instantaneous excitation
jump and exponential relaxation. Sampling density follows horizontal pixels,
so the curve is smooth without a fixed temporal window or arbitrary step count.
*/
export const hawkesTrace = (
	samples: HawkesTraceSample[],
	horizontalPixels: number,
): HawkesTracePoint[] => {
	if (samples.length === 0) {
		return [];
	}

	const first = samples[0] as HawkesTraceSample;
	const last = samples[samples.length - 1] as HawkesTraceSample;
	const span = last.at > first.at ? last.at - first.at : 1n;
	const trace: HawkesTracePoint[] = [
		{ at: first.at, intensity: first.intensity },
	];

	for (let index = 0; index < samples.length; index++) {
		const current = samples[index] as HawkesTraceSample;
		trace.push({ at: current.at, intensity: current.postArrival });
		const next = samples[index + 1];

		if (!next || next.at <= current.at) {
			continue;
		}

		const gap = next.at - current.at;
		const steps = Math.max(
			1,
			Math.ceil((Number(gap) / Number(span)) * horizontalPixels),
		);
		const gapSeconds = Number(gap) / 1e9;

		for (let step = 1; step <= steps; step++) {
			const fraction = step / steps;
			const seconds = gapSeconds * fraction;
			trace.push({
				at: current.at + BigInt(Math.round(Number(gap) * fraction)),
				intensity:
					current.baseline +
					(current.postArrival - current.baseline) *
						Math.exp(-current.decay * seconds),
			});
		}

		trace.push({ at: next.at, intensity: next.intensity });
	}

	return trace;
};
