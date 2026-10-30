import { s3KeyFromIndex, thresholdFromExploreBytes } from "#/collections/cognition";
import { framePositionRows } from "#/collections/positions";
import {
	EXPLORE_BASE_THRESHOLD,
	filterExploreKeys,
	pathBookPolicyKey,
} from "#/components/learning/explore-path";
import { parseOneValue, type WireValue } from "#/types/decoder";

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

export const isS3ListPrefix = (wire: WireValue): boolean =>
	wire.origin === "s3" && wire.role === "prefix";

const isClosedListScope = (scope: string): boolean =>
	scope === "position/closed" ||
	scope === "virtual/closed" ||
	scope.startsWith("position/closed/") ||
	scope.startsWith("virtual/closed/");

const isExploreListScope = (scope: string): boolean =>
	scope === "model/paths" || scope.startsWith("model/paths/");

export const compactS3ListPrefix = (
	wire: WireValue,
): S3PrefixCompact | null => {
	if (!isS3ListPrefix(wire)) {
		return null;
	}

	if (isClosedListScope(wire.scope)) {
		return compactClosedHistory(wire);
	}

	if (isExploreListScope(wire.scope)) {
		return compactExplorePaths(wire);
	}

	return null;
};

const compactClosedHistory = (wire: WireValue): S3ClosedCompact => {
	const entries: ClosedHistoryEntry[] = [];

	for (const layer of wire.layers) {
		const nested = parseOneValue(layer);
		if (!nested) {
			continue;
		}

		if (nested.role !== "closed") {
			continue;
		}

		if (nested.origin !== "position" && nested.origin !== "virtual") {
			continue;
		}

		const row = framePositionRows(nested)[0];
		if (!row || typeof row.id !== "string" || row.id === "") {
			continue;
		}

		entries.push({
			stamp: {
				epoch: nested.epoch ?? 0n,
				tick: nested.tick ?? 0n,
				timestamp: nested.timestamp ?? 0n,
			},
			row,
		});
	}

	return { kind: "closed", scope: wire.scope, entries };
};

const compactExplorePaths = (wire: WireValue): S3ExploreCompact => {
	const policyKeys: string[] = [];
	const thresholds: Record<string, number> = {};

	for (let i = 0; i < wire.index.length; i++) {
		const rawKey = s3KeyFromIndex(wire.prefix, wire.index[i]);
		if (rawKey === null || !rawKey.startsWith(`${wire.scope}/`)) {
			continue;
		}

		const key = pathBookPolicyKey(rawKey, wire.layers[i]);
		if (!policyKeys.includes(key)) {
			policyKeys.push(key);
		}

		const threshold = thresholdFromExploreBytes(wire.layers[i]);
		if (threshold !== null) {
			thresholds[key] = threshold;
			continue;
		}

		thresholds[key] = EXPLORE_BASE_THRESHOLD;
	}

	policyKeys.sort();

	const keptKeys = filterExploreKeys(policyKeys, thresholds);
	const keptThresholds: Record<string, number> = {};
	for (const key of keptKeys) {
		const threshold = thresholds[key];
		if (threshold !== undefined) {
			keptThresholds[key] = threshold;
		}
	}

	const epoch =
		wire.epoch != null && wire.epoch !== 0n ? String(wire.epoch) : null;

	return {
		kind: "explore",
		scope: wire.scope,
		epoch,
		policyKeys: keptKeys,
		thresholds: keptThresholds,
	};
};

/*
partitionWireBatch keeps ListPrefix frames off the main thread payload. Their
layers are folded into S3PrefixCompact records in the worker.
*/
export const partitionWireBatch = (
	wires: WireValue[],
): { values: WireValue[]; s3: S3PrefixCompact[] } => {
	const values: WireValue[] = [];
	const s3: S3PrefixCompact[] = [];

	for (const wire of wires) {
		const compact = compactS3ListPrefix(wire);
		if (compact) {
			s3.push(compact);
			continue;
		}

		values.push(wire);
	}

	return { values, s3 };
};
