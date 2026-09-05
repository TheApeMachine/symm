import type {
	HindsightTimeline,
	HindsightTimelineBucket,
} from "./hindsight-types";

/*
The timeline's horizontal scale.

Whatever axis is displayed, everything on the timeline is addressed by
CaptureSequence: an episode, a reference point, a reconnect, and the playhead
are all capture identities, and only their pixel position depends on the axis.
So the scale is built as a monotone sequence→pixel map anchored on the buckets
the projection already computed, rather than as two separate axis formulas that
could disagree about where a capture sits.
*/

export type TimelineScale = {
	/* Pixel x of one capture sequence, clamped to the plotted window. */
	xOf: (sequence: number) => number;
	/* The capture sequence nearest a pixel x, always a sequence that was observed. */
	sequenceAt: (x: number) => number;
	/* The bucket a pixel x falls in, or null outside the plotted window. */
	bucketAt: (x: number) => HindsightTimelineBucket | null;
	/* Pixel width of one bucket. */
	step: number;
	width: number;
};

type Anchor = {
	sequence: number;
	x: number;
};

/*
buildScale derives the anchors from the buckets' observed spans — the capture
range each bucket actually covered — so the map is pinned to real observations
rather than to the nominal edges the axis drew. A bucket that observed nothing
contributes no anchor and is simply spanned by its neighbours.
*/
export const buildScale = (
	timeline: HindsightTimeline | null,
	width: number,
): TimelineScale => {
	const buckets = timeline?.buckets ?? [];
	const step = buckets.length > 0 ? width / buckets.length : width;
	const anchors: Anchor[] = [];

	for (const bucket of buckets) {
		if (bucket.observations === 0) continue;

		const left = bucket.index * step;

		if (bucket.observedFromSequence > 0) {
			anchors.push({ sequence: bucket.observedFromSequence, x: left });
		}

		if (bucket.observedToSequence > bucket.observedFromSequence) {
			anchors.push({
				sequence: bucket.observedToSequence,
				x: left + step,
			});
		}
	}

	if (anchors.length === 0) {
		const from = timeline?.span.fromSequence ?? 0;
		const to = timeline?.span.toSequence ?? from + 1;
		const range = Math.max(to - from, 1);

		return {
			xOf: (sequence) =>
				Math.min(width, Math.max(0, ((sequence - from) / range) * width)),
			sequenceAt: (x) =>
				from + (Math.min(Math.max(x, 0), width) / width) * range,
			bucketAt: () => null,
			step,
			width,
		};
	}

	const first = anchors[0];
	const last = anchors[anchors.length - 1];

	const xOf = (sequence: number): number => {
		if (sequence <= first.sequence) return first.x;
		if (sequence >= last.sequence) return last.x;

		let low = 0;
		let high = anchors.length - 1;

		while (high - low > 1) {
			const mid = (low + high) >> 1;

			if (anchors[mid].sequence <= sequence) {
				low = mid;
			} else {
				high = mid;
			}
		}

		const span = anchors[high].sequence - anchors[low].sequence;

		if (span <= 0) return anchors[low].x;

		const fraction = (sequence - anchors[low].sequence) / span;

		return anchors[low].x + fraction * (anchors[high].x - anchors[low].x);
	};

	const bucketAt = (x: number): HindsightTimelineBucket | null => {
		if (buckets.length === 0) return null;

		const index = Math.min(
			buckets.length - 1,
			Math.max(0, Math.floor(x / step)),
		);

		return buckets[index] ?? null;
	};

	const sequenceAt = (x: number): number => {
		const bucket = bucketAt(x);

		if (bucket === null || bucket.observations === 0) {
			// Fall back to the nearest bucket that observed anything, so a click
			// on a quiet stretch still resolves to a real captured frame rather
			// than to an interpolated sequence that was never observed.
			let nearest: HindsightTimelineBucket | null = null;
			let distance = Number.POSITIVE_INFINITY;

			for (const candidate of buckets) {
				if (candidate.observations === 0) continue;

				const centre = (candidate.index + 0.5) * step;
				const gap = Math.abs(centre - x);

				if (gap < distance) {
					distance = gap;
					nearest = candidate;
				}
			}

			return nearest?.observedFromSequence ?? first.sequence;
		}

		const offset = (x - bucket.index * step) / step;

		return offset < 0.5
			? bucket.observedFromSequence
			: bucket.observedToSequence || bucket.observedFromSequence;
	};

	return { xOf, sequenceAt, bucketAt, step, width };
};

/*
The vertical scale of the coordinate track. Only buckets where the declared
coordinate was actually defined contribute to the extent — an unmeasured bucket
must never be read as a value of zero, which would drag the whole track to the
floor and invent a crash that never happened.
*/
export type ValueScale = {
	yOf: (value: number) => number;
	low: number;
	high: number;
	defined: boolean;
};

export const buildValueScale = (
	buckets: HindsightTimelineBucket[],
	height: number,
	padding = 6,
): ValueScale => {
	let low = Number.POSITIVE_INFINITY;
	let high = Number.NEGATIVE_INFINITY;

	for (const bucket of buckets) {
		if (!bucket.defined) continue;

		if (bucket.low < low) low = bucket.low;
		if (bucket.high > high) high = bucket.high;
	}

	if (!Number.isFinite(low) || !Number.isFinite(high)) {
		return { yOf: () => height / 2, low: 0, high: 0, defined: false };
	}

	const range = high - low || Math.abs(high) * 1e-6 || 1;
	const usable = height - padding * 2;

	return {
		yOf: (value) => padding + (1 - (value - low) / range) * usable,
		low,
		high,
		defined: true,
	};
};

/*
formatPrice keeps a coordinate readable across the four orders of magnitude the
universe spans, without ever rounding a small-tick instrument into a flat line.
*/
export const formatPrice = (value: number): string => {
	if (!Number.isFinite(value)) return "—";

	const magnitude = Math.abs(value);

	if (magnitude >= 1000) return value.toFixed(1);
	if (magnitude >= 10) return value.toFixed(3);
	if (magnitude >= 0.1) return value.toFixed(5);

	return value.toPrecision(5);
};

/*
formatClock renders an instant on a 24-hour clock, and refuses the Go zero time
outright: a bucket that observed nothing carries year 0001, and formatting that
as a wall-clock time would put a fabricated timestamp on the axis.
*/
export const formatClock = (iso: string): string => {
	if (!iso) return "—";

	const at = new Date(iso);

	if (Number.isNaN(at.getTime()) || at.getUTCFullYear() < 1970) return "—";

	return at.toLocaleTimeString([], {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		hour12: false,
	});
};

/*
observationGap is the mean interval between this instrument's observations over
the plotted window. It is the yardstick the track uses to decide whether a hole
is quantisation or a span that genuinely went unobserved.
*/
export const observationGap = (
	buckets: HindsightTimelineBucket[],
): number | null => {
	let first = Number.POSITIVE_INFINITY;
	let last = Number.NEGATIVE_INFINITY;
	let count = 0;

	for (const bucket of buckets) {
		if (bucket.observations === 0) continue;

		count += bucket.observations;

		const from = new Date(bucket.observedFromAt).getTime();
		const to = new Date(bucket.observedToAt).getTime();

		if (Number.isNaN(from) || Number.isNaN(to)) continue;

		if (from < first) first = from;
		if (to > last) last = to;
	}

	if (count < 2 || !Number.isFinite(first) || !Number.isFinite(last)) {
		return null;
	}

	const span = last - first;

	return span > 0 ? span / count : null;
};

export const formatCount = (value: number): string =>
	value.toLocaleString(undefined, { maximumFractionDigits: 0 });
