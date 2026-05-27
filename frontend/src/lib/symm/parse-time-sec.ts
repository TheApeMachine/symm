export const parseTimeSec = (raw: unknown): number | undefined => {
	if (typeof raw !== "string") {
		return undefined;
	}

	const parsed = Date.parse(raw);

	if (!Number.isFinite(parsed)) {
		return undefined;
	}

	return parsed / 1000;
};
