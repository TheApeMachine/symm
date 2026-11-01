import {
	EXPLORE_BASE_THRESHOLD,
	filterExploreKeys,
} from "#/components/learning/explore-path";

export type ClosedHistoryEntry = {
	stamp: { epoch: bigint; tick: bigint; timestamp: bigint };
	row: Record<string, unknown>;
};

export type S3ClosedCompact = {
	kind: "closed";
	scope: string;
	entries: ClosedHistoryEntry[];
};

export type S3ExploreCompact = {
	kind: "explore";
	scope: string;
	epoch: string | null;
	policyKeys: string[];
	thresholds: Record<string, number>;
};

export type S3PrefixCompact = S3ClosedCompact | S3ExploreCompact;

const asFiniteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const asBigInt = (value: unknown): bigint => {
	if (typeof value === "bigint") return value;
	if (typeof value === "number" && Number.isFinite(value)) {
		return BigInt(Math.trunc(value));
	}
	if (typeof value === "string" && value !== "") {
		try {
			return BigInt(value);
		} catch {
			return 0n;
		}
	}
	return 0n;
};

/*
pathsListingFromEnvelope folds hub WriteJSON {"paths":{…}} into the explore
compact the cognitive tree already ingests.
*/
export const pathsListingFromEnvelope = (
	payload: unknown,
): S3ExploreCompact | null => {
	if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
		return null;
	}

	const body = payload as {
		scope?: unknown;
		epoch?: unknown;
		thresholds?: unknown;
		policy_keys?: unknown;
		policyKeys?: unknown;
	};

	const scope = typeof body.scope === "string" ? body.scope : "";
	if (scope !== "model/paths" && !scope.startsWith("model/paths/")) {
		return null;
	}

	const thresholds: Record<string, number> = {};
	if (body.thresholds && typeof body.thresholds === "object") {
		for (const [key, raw] of Object.entries(
			body.thresholds as Record<string, unknown>,
		)) {
			const value = asFiniteNumber(raw);
			thresholds[key] = value ?? EXPLORE_BASE_THRESHOLD;
		}
	}

	const listedKeys = Array.isArray(body.policy_keys)
		? body.policy_keys
		: Array.isArray(body.policyKeys)
			? body.policyKeys
			: Object.keys(thresholds);

	const policyKeys: string[] = [];
	for (const key of listedKeys) {
		if (typeof key !== "string" || key === "") continue;
		if (!(key in thresholds)) thresholds[key] = EXPLORE_BASE_THRESHOLD;
		policyKeys.push(key);
	}

	policyKeys.sort();

	const keptKeys = filterExploreKeys(policyKeys, thresholds);
	const keptThresholds: Record<string, number> = {};
	for (const key of keptKeys) {
		const threshold = thresholds[key];
		if (threshold !== undefined) keptThresholds[key] = threshold;
	}

	const epoch =
		typeof body.epoch === "string" && body.epoch !== ""
			? body.epoch
			: typeof body.epoch === "number" && Number.isFinite(body.epoch)
				? String(Math.trunc(body.epoch))
				: null;

	return {
		kind: "explore",
		scope: "model/paths",
		epoch,
		policyKeys: keptKeys,
		thresholds: keptThresholds,
	};
};

/*
closedListingFromEnvelope folds hub WriteJSON {"closed":{…}} into history
entries without decoding data.Value blobs.
*/
export const closedListingFromEnvelope = (
	payload: unknown,
): S3ClosedCompact | null => {
	if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
		return null;
	}

	const body = payload as { scope?: unknown; entries?: unknown };
	const scope = typeof body.scope === "string" ? body.scope : "";
	if (
		scope !== "position/closed" &&
		scope !== "virtual/closed" &&
		!scope.startsWith("position/closed/") &&
		!scope.startsWith("virtual/closed/")
	) {
		return null;
	}

	if (!Array.isArray(body.entries)) {
		return { kind: "closed", scope, entries: [] };
	}

	const entries: ClosedHistoryEntry[] = [];
	for (const raw of body.entries) {
		if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;

		const entry = raw as {
			epoch?: unknown;
			tick?: unknown;
			timestamp?: unknown;
			row?: unknown;
		};

		if (!entry.row || typeof entry.row !== "object" || Array.isArray(entry.row)) {
			continue;
		}

		const row = entry.row as Record<string, unknown>;
		if (typeof row.id !== "string" || row.id === "") continue;

		entries.push({
			stamp: {
				epoch: asBigInt(entry.epoch),
				tick: asBigInt(entry.tick),
				timestamp: asBigInt(entry.timestamp),
			},
			row,
		});
	}

	return { kind: "closed", scope, entries };
};
