export const finiteCount = (value: unknown): number | null => {
	if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
		return null;
	}

	return Math.floor(value);
};

export type PlaybookBranchWire = {
	branches?: PlaybookBranchWire[];
	[key: string]: unknown;
};

export const isPlaybookBranch = (
	value: unknown,
): value is PlaybookBranchWire => {
	if (typeof value !== "object" || value === null) {
		return false;
	}

	const branch = value as PlaybookBranchWire;

	if (Array.isArray(branch.branches)) {
		return branch.branches.every(isPlaybookBranch);
	}

	return true;
};

export const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

export const gaugeFramesFromState = (
	raw: Record<string, unknown>,
): Record<string, unknown>[] => {
	const gaugeReadings = raw.gauge_readings;

	if (Array.isArray(gaugeReadings)) {
		return gaugeReadings.filter(
			(frame): frame is Record<string, unknown> =>
				typeof frame === "object" && frame !== null,
		);
	}

	const measurements = raw.measurements;

	if (!Array.isArray(measurements)) {
		return [];
	}

	return measurements.filter(
		(frame): frame is Record<string, unknown> =>
			typeof frame === "object" && frame !== null,
	);
};

export const decisionTreeBranches = (
	raw: Record<string, unknown>,
): PlaybookBranchWire[] | null => {
	const topLevel = raw.branches;

	if (Array.isArray(topLevel) && topLevel.every(isPlaybookBranch)) {
		return topLevel;
	}

	const nested = raw.value;

	if (typeof nested === "object" && nested !== null) {
		const nestedBranches = (nested as Record<string, unknown>).branches;

		if (
			Array.isArray(nestedBranches) &&
			nestedBranches.every(isPlaybookBranch)
		) {
			return nestedBranches;
		}
	}

	return null;
};
