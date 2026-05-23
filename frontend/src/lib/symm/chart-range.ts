export function candleYRange(
	seriesMin: number,
	seriesMax: number,
	livePrices: number[],
): { min: number; max: number } | null {
	const live = livePrices.filter(
		(value) => Number.isFinite(value) && value > 0,
	);

	let min = seriesMin;
	let max = seriesMax;
	const seriesValid = Number.isFinite(min) && Number.isFinite(max) && min < max;

	if (!seriesValid) {
		if (live.length === 0) {
			return null;
		}

		min = Math.min(...live);
		max = Math.max(...live);
	} else if (live.length > 0) {
		min = Math.min(min, ...live);
		max = Math.max(max, ...live);
	}

	if (min >= max) {
		const mid = live.length > 0 ? live[live.length - 1] : min;
		const pad = Math.max(Math.abs(mid) * 0.002, 1e-8);
		return { min: mid - pad, max: mid + pad };
	}

	const mid = (min + max) / 2;
	const span = Math.max(max - min, Math.abs(mid) * 1e-4, 1e-8);
	const pad = span * 0.15;

	return { min: min - pad, max: max + pad };
}

export class PulseValueClip {
	private readonly history: number[] = [];

	constructor(private readonly cap = 64) {}

	clip(value: number): number {
		if (!Number.isFinite(value)) {
			return 0;
		}

		const fence = this.fence();

		if (fence <= 0) {
			this.record(value);
			return value;
		}

		const clipped = Math.max(-fence, Math.min(fence, value));
		this.record(clipped);

		return clipped;
	}

	private fence(): number {
		if (this.history.length < 8) {
			return 0;
		}

		const sorted = [...this.history].sort((left, right) => left - right);
		const median = sorted[Math.floor(sorted.length / 2)];
		const deviations = sorted
			.map((value) => Math.abs(value - median))
			.sort((left, right) => left - right);
		const mad = deviations[Math.floor(deviations.length / 2)];
		const spread = Math.max(mad * 3, Math.abs(median) * 0.5, 1e-6);

		return Math.abs(median) + spread;
	}

	private record(value: number) {
		this.history.push(value);

		if (this.history.length > this.cap) {
			this.history.shift();
		}
	}
}

export function pulseYRange(
	reValues: number[],
	turbValues: number[],
	divValues: number[],
): { min: number; max: number } | null {
	const values = [...reValues, ...turbValues, ...divValues].filter((value) =>
		Number.isFinite(value),
	);

	if (values.length === 0) {
		return null;
	}

	const min = Math.min(...values);
	const max = Math.max(...values);

	if (min === max) {
		const pad = Math.max(Math.abs(min) * 0.1, 0.01);
		return { min: min - pad, max: max + pad };
	}

	const span = max - min;
	const pad = span * 0.12;

	return { min: min - pad, max: max + pad };
}
