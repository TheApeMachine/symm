/*
Tape ticks on a closed desk lot index workspace/tick/aggregate/<epoch>/<tick>.
A is the entry tick, B entry, C exit (no separate precursor mark on the lot).
*/
export const tickUint = (value: unknown): number | null => {
	if (typeof value === "number" && Number.isFinite(value) && value > 0) {
		return value;
	}

	if (typeof value === "string" && /^\d+$/.test(value.trim())) {
		return Number(value.trim());
	}

	return null;
};

export const policyEpochFromRow = (row: {
	policy_epoch?: number | string | null;
}): string | undefined => {
	const epoch = row.policy_epoch;
	if (epoch === undefined || epoch === null || epoch === "") {
		return undefined;
	}

	return String(epoch);
};

export const tapeMarksFromDeskRow = (row: {
	entry_tick?: number | string | null;
	exit_tick?: number | string | null;
}): { a: number; b: number; c: number } | null => {
	const entry = tickUint(row.entry_tick);
	const exit = tickUint(row.exit_tick);
	if (entry === null || exit === null || exit < entry) {
		return null;
	}

	return { a: entry, b: entry, c: exit };
};
