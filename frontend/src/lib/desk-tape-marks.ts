/*
Tape ticks on a closed desk lot index workspace/tick/aggregate/<epoch>/<tick>.
A is the precursor anchor (mark_a_tick when set, else entry), B entry, C exit.
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

export const policyEpochFromRow = (
	row: Record<string, unknown>,
): string | undefined => {
	const epoch = row.policy_epoch ?? row.run;
	if (epoch === undefined || epoch === null || epoch === "") {
		return undefined;
	}

	return String(epoch);
};

export const tapeMarksFromDeskRow = (
	row: Record<string, unknown>,
): { a: number; b: number; c: number } | null => {
	const entry = tickUint(row.entry_tick);
	const exit = tickUint(row.exit_tick);
	if (entry === null || exit === null || exit < entry) {
		return null;
	}

	const legacy = row.marks as Record<string, unknown> | undefined;
	const legacyA = tickUint(legacy?.a);
	const legacyB = tickUint(legacy?.b);
	const legacyC = tickUint(legacy?.c);
	if (legacyA !== null && legacyB !== null && legacyC !== null) {
		return { a: legacyA, b: legacyB, c: legacyC };
	}

	const markA = tickUint(row.mark_a_tick) ?? entry;
	const a = markA <= entry ? markA : entry;

	return { a, b: entry, c: exit };
};
