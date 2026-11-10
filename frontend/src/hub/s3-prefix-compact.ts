import type { Position } from "#/collections/types";
import { filterExploreKeys } from "#/components/learning/explore-path";
import { asBigInt, asFinite } from "#/lib/format";
import type { ClosedHistoryEntry } from "#/lib/lifecycle";

export type { ClosedHistoryEntry };

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

/*
pathsListingFromEnvelope folds hub WriteJSON {"pathbook":{…}} into the explore
compact the cognitive tree already ingests. Thresholds are absolute PathBook
resistance values — missing keys stay missing (no invented 0.5).
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
			const value = asFinite(raw);
			if (value === null) continue;
			thresholds[key] = value;
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
entries from lot JSON {epoch,tick,timestamp,position→row}.
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
		scope !== "paper/closed" &&
		scope !== "real/closed" &&
		scope !== "virtual/closed" &&
		!scope.startsWith("paper/closed/") &&
		!scope.startsWith("real/closed/") &&
		!scope.startsWith("virtual/closed/")
	) {
		return null;
	}

	if (!Array.isArray(body.entries)) {
		return { kind: "closed", scope, entries: [] };
	}

	const book = bookFromClosedScope(scope);

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

		const row = entry.row as Position;
		if (typeof row.id !== "string" || row.id === "") continue;

		// Position JSON may omit tier_key — stamp it from the S3 tier prefix.
		const stamped: Position =
			typeof row.tier_key === "string" && row.tier_key !== ""
				? row
				: { ...row, tier_key: book };

		entries.push({
			stamp: {
				epoch: asBigInt(entry.epoch),
				tick: asBigInt(entry.tick),
				timestamp: asBigInt(entry.timestamp),
			},
			row: stamped,
		});
	}

	return { kind: "closed", scope, entries };
};

const bookFromClosedScope = (scope: string): "paper" | "virtual" | "real" => {
	if (scope.startsWith("virtual/")) return "virtual";
	if (scope.startsWith("real/")) return "real";
	return "paper";
};
