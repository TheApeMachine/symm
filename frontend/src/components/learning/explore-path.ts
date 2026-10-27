/** Desk baseline before any settled lot adjusts the leaf. */
export const EXPLORE_BASE_THRESHOLD = 0.5;

/** Matches strategy Model.updatePathThreshold: threshold > 0.8 is deleted. */
export const EXPLORE_PRUNE_THRESHOLD = 0.8;

export const exploreThresholdForKey = (
	key: string,
	thresholds?: Record<string, number> | null,
): number | undefined => {
	if (!key.startsWith("model/explore/")) return undefined;
	const raw = thresholds?.[key];
	if (typeof raw === "number" && Number.isFinite(raw)) return raw;
	return EXPLORE_BASE_THRESHOLD;
};

export const explorePathKept = (
	key: string,
	thresholds?: Record<string, number> | null,
): boolean => {
	if (!key.startsWith("model/explore/")) return true;
	const threshold = exploreThresholdForKey(key, thresholds);
	return (threshold ?? EXPLORE_BASE_THRESHOLD) <= EXPLORE_PRUNE_THRESHOLD;
};

export const filterExploreKeys = (
	keys: string[],
	thresholds?: Record<string, number> | null,
): string[] => keys.filter((key) => explorePathKept(key, thresholds));
