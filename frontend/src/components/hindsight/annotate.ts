/*
A hand-placed detection on the Hindsight tape. A, B, C and D are workspace
ticks the chart already shows. The class is the detection's role.
*/

export const MARKERS = ["A", "B", "C", "D"] as const;

export type Marker = (typeof MARKERS)[number];

export const DETECTION_CLASSES = ["up", "down", "chop", "flat"] as const;

export type DetectionClass = (typeof DETECTION_CLASSES)[number];

export type Annotation = Record<Marker, number | null>;

export const emptyAnnotation = (): Annotation => ({
	A: null,
	B: null,
	C: null,
	D: null,
});

/*
snapTick is the stored tick nearest the click. The chart's axis is tick
numbers, and a click can land between two of them.
*/
export const snapTick = (ticks: number[], sequence: number): number | null => {
	if (ticks.length === 0 || !Number.isFinite(sequence)) return null;

	let best = ticks[0];
	let bestDist = Math.abs(ticks[0] - sequence);

	for (const tick of ticks) {
		const dist = Math.abs(tick - sequence);
		if (dist < bestDist) {
			best = tick;
			bestDist = dist;
		}
	}

	return best;
};

export const placeMarker = (
	annotation: Annotation,
	marker: Marker,
	tick: number,
): Annotation => ({ ...annotation, [marker]: tick });

export const nextMarker = (current: Marker): Marker =>
	MARKERS[(MARKERS.indexOf(current) + 1) % MARKERS.length];

/*
draftOf is the working prediction of cls with the widest price move. The
committed copies live under detections/ and are not what the markers drag.
*/
export const draftOf = (
	excursions: {
		kind: string;
		key: string;
		fields: {
			b_price: number;
			c_price: number;
			a_tick: number;
			b_tick: number;
			c_tick: number;
			d_tick: number;
		};
	}[],
	cls: DetectionClass,
) => {
	const matches = excursions.filter(
		(excursion) => excursion.kind === cls && excursion.key.startsWith("draft/"),
	);

	return (
		matches.sort(
			(left, right) =>
				Math.abs(right.fields.c_price - right.fields.b_price) -
				Math.abs(left.fields.c_price - left.fields.b_price),
		)[0] ?? null
	);
};

export const marksOf = (
	annotation: Annotation,
): { label: Marker; at: number }[] =>
	MARKERS.flatMap((label) => {
		const at = annotation[label];
		return at === null ? [] : [{ label, at }];
	});

/*
annotationBody is the save payload. Null when a marker is missing or the
ticks are out of tape order.
*/
export const annotationBody = (
	run: string,
	symbol: string,
	cls: DetectionClass,
	annotation: Annotation,
): {
	run: string;
	symbol: string;
	class: DetectionClass;
	a: string;
	b: string;
	c: string;
	d: string;
} | null => {
	const { A, B, C, D } = annotation;
	if (A === null || B === null || C === null || D === null) return null;
	if (!(A <= B && B <= C && C <= D)) return null;

	return {
		run,
		symbol,
		class: cls,
		a: String(A),
		b: String(B),
		c: String(C),
		d: String(D),
	};
};
