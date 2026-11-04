/** Desk baseline written into a brand-new PathBook.resistance. */
export const EXPLORE_BASE_THRESHOLD = 0.5;

const PATH_ACTIONS = new Set(["enter", "hold", "exit"]);

/*
pathBookPolicyKey maps an S3 object key to the trie key RadixTreeViz expects.
Tree.persist writes model/paths/<tokens>.bin with action in JSON; rewrite to
…/enter|hold|exit.bin. Canonical action-suffixed keys pass through.
*/
export const pathBookPolicyKey = (
	storageKey: string,
	action?: string,
): string => {
	if (
		storageKey.endsWith("/enter.bin") ||
		storageKey.endsWith("/hold.bin") ||
		storageKey.endsWith("/exit.bin")
	) {
		return storageKey;
	}

	if (action && PATH_ACTIONS.has(action)) {
		const base = storageKey.endsWith(".bin")
			? storageKey.slice(0, -".bin".length)
			: storageKey;
		return `${base}/${action}.bin`;
	}

	return storageKey;
};

/** Matches strategy Tree.Predict: resistance > 0.8 is deleted. */
export const EXPLORE_PRUNE_THRESHOLD = 0.8;

const PATH_BOOKS = ["model/paths"] as const;

export const isPathBookKey = (key: string) =>
	PATH_BOOKS.some((prefix) => key.startsWith(prefix));

/*
exploreThresholdForKey returns the stored absolute book, or undefined when
the hub never reported one. Never invent the 0.5 seed for missing keys.
*/
export const exploreThresholdForKey = (
	key: string,
	thresholds?: Record<string, number> | null,
): number | undefined => {
	if (!isPathBookKey(key)) return undefined;
	const raw = thresholds?.[key];
	if (typeof raw === "number" && Number.isFinite(raw)) return raw;
	return undefined;
};

export const explorePathKept = (
	key: string,
	thresholds?: Record<string, number> | null,
): boolean => {
	if (!isPathBookKey(key)) return true;
	const threshold = exploreThresholdForKey(key, thresholds);
	if (threshold === undefined) return true;
	return threshold <= EXPLORE_PRUNE_THRESHOLD;
};

export const filterExploreKeys = (
	keys: string[],
	thresholds?: Record<string, number> | null,
): string[] => keys.filter((key) => explorePathKept(key, thresholds));
