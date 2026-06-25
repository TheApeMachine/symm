export const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

export const whyLabel = (value: string | undefined) => {
	if (value === undefined || value.trim() === "") {
		return "—";
	}

	return value.replaceAll("_", " ");
};

export const fixed = (value: number, digits = 3): string => {
	if (!Number.isFinite(value)) {
		return "0";
	}

	return value.toFixed(digits);
};

const sortedFiniteScores = (scores: number[]) =>
	scores.filter(Number.isFinite).sort((left, right) => left - right);

export const entryLineStats = (scores: number[]) => {
	const finite = sortedFiniteScores(scores);

	if (finite.length === 0) {
		return { median: 0, mad: 0, line: 0, linePercent: 0 };
	}

	const median = finite[Math.floor(finite.length / 2)] ?? 0;
	const mad =
		finite.reduce((sum, score) => sum + Math.abs(score - median), 0) /
		finite.length;
	const line = median + 0.55 * mad + 0.06;

	return {
		median,
		mad,
		line,
		linePercent: clamp(line * 100, 0, 100),
	};
};

/** Matches frontend/tmp allocation x-ray: threshold = median + mad. */
export const allocationEntryStats = (scores: number[]) => {
	const finite = sortedFiniteScores(scores);

	if (finite.length === 0) {
		return { median: 0, mad: 0, threshold: 0 };
	}

	const median = finite[Math.floor(finite.length / 2)] ?? 0;
	const mad =
		finite.reduce((sum, score) => sum + Math.abs(score - median), 0) /
		finite.length;

	return {
		median,
		mad,
		threshold: median + mad,
	};
};
