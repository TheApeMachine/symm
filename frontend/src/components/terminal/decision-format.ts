/*
decision-format is production-safe only for literal formatting helpers such as
fixed and whyLabel. The score-distribution functions below are legacy fixture
helpers retained for non-live tests; live decision and allocation surfaces must
read backend entry statistics and verdicts directly.
*/

const sorted = (values: number[]): number[] =>
	[...values].sort((left, right) => left - right);

/*
median returns the middle value (mean of the two middle values for even length).
*/
export const median = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const order = sorted(values);
	const mid = Math.floor(order.length / 2);

	return order.length % 2 === 0
		? (order[mid - 1] + order[mid]) / 2
		: order[mid];
};

/*
mad is the median absolute deviation from the median — a robust spread that the
entry gate adds to the median so only standout candidates clear it.
*/
export const mad = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const center = median(values);

	return median(values.map((value) => Math.abs(value - center)));
};

/*
fixed formats a score to three decimals, the precision the tmp terminal used for
combined scores and edges.
*/
export const fixed = (value: number | string): string => {
	const numeric = Number(value);

	if (!Number.isFinite(numeric)) return "—";
	if (numeric === 0) return "0.000";
	// Sub-cent prices need more precision to avoid showing 0.000
	if (Math.abs(numeric) < 0.1) return numeric.toFixed(6);
	if (Math.abs(numeric) < 1.0) return numeric.toFixed(4);
	return numeric.toFixed(3);
};

/*
entryLineStats is a non-live legacy helper for tests/fixtures. Production
decision surfaces must not derive trader gates from frontend score arrays.
*/
// Robust standout factor: median + 1.5×MAD marks a candidate as clearing the
// gate, mirroring the ~1.5 MAD outlier convention. Only scores meaningfully above
// the pack qualify, so the gate adapts to the live distribution's spread.
const STANDOUT_MAD = 1.5;

export const entryLineStats = (
	scores: number[],
): { line: number; median: number; mad: number } => {
	const med = median(scores);
	const dispersion = mad(scores);

	// The gate is median + 1.5×MAD, but it can never exceed the best candidate's
	// score: a tight distribution must not veto its own top opportunity. Clamping
	// to the max keeps the single strongest candidate deployable while still
	// rejecting the pack below it.
	const ceiling = scores.length > 0 ? Math.max(...scores) : 0;
	const line = Math.min(med + STANDOUT_MAD * dispersion, ceiling);

	return { line, median: med, mad: dispersion };
};

/*
allocationEntryStats mirrors the tmp allocation x-ray. It intentionally uses the
upper middle score for even sets and the mean absolute deviation from that
center, matching the functional mockup's sizing gate.
*/
export const allocationEntryStats = (
	scores: number[],
): { threshold: number; median: number; mad: number } => {
	if (scores.length === 0) {
		return { threshold: 0, median: 0, mad: 0 };
	}

	const order = sorted(scores);
	const med = order[Math.floor(order.length / 2)];
	const dispersion =
		order.reduce((sum, score) => sum + Math.abs(score - med), 0) / order.length;

	return { threshold: med + dispersion, median: med, mad: dispersion };
};

const REASON_LABELS: Record<string, string> = {
	matched_branch: "matched branch",
	below_entry: "below entry line",
	below_edge: "below edge",
	held: "already held",
	no_slot: "no slot",
	no_causal_uplift: "no causal uplift",
	no_manifold_field: "no manifold field",
};

/*
whyLabel turns a raw walk/decider reason token into the human phrase the surface
shows. Unknown reasons pass through with underscores normalised; an empty reason
renders as an em dash so a missing reason is visible, not blank.
*/
export const whyLabel = (reason?: string): string => {
	if (reason === undefined || reason.trim() === "") {
		return "—";
	}

	return REASON_LABELS[reason] ?? reason.replace(/_/g, " ");
};
