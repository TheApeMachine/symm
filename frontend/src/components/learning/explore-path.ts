/** Desk baseline before any settled lot adjusts the leaf. */
export const EXPLORE_BASE_THRESHOLD = 0.5;

/** Matches strategy Model.updatePathThreshold: threshold > 0.8 is deleted. */
export const EXPLORE_PRUNE_THRESHOLD = 0.8;

const PATH_BOOKS = ["model/path/"] as const;

export const isPathBookKey = (key: string) =>
	PATH_BOOKS.some((prefix) => key.startsWith(prefix));

export const exploreThresholdForKey = (
	key: string,
	thresholds?: Record<string, number> | null,
): number | undefined => {
	if (!isPathBookKey(key)) return undefined;
	const raw = thresholds?.[key];
	if (typeof raw === "number" && Number.isFinite(raw)) return raw;
	return EXPLORE_BASE_THRESHOLD;
};

export const explorePathKept = (
	key: string,
	thresholds?: Record<string, number> | null,
): boolean => {
	if (!isPathBookKey(key)) return true;
	const threshold = exploreThresholdForKey(key, thresholds);
	return (threshold ?? EXPLORE_BASE_THRESHOLD) <= EXPLORE_PRUNE_THRESHOLD;
};

export const filterExploreKeys = (
	keys: string[],
	thresholds?: Record<string, number> | null,
): string[] => keys.filter((key) => explorePathKept(key, thresholds));
