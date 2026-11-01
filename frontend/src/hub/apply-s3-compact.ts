import { ingestExploreListPrefix } from "#/collections/cognition";
import { seedFromClosedLots } from "#/collections/longtail";
import {
	closedListingFromEnvelope,
	pathsListingFromEnvelope,
} from "#/hub/s3-prefix-compact";
import { ingestHistoryClosedBatch } from "#/lib/lifecycle";

/*
applyPathsEnvelope / applyClosedEnvelope ingest hub JSON ListPrefix listings.
*/
export const applyPathsEnvelope = (payload: unknown): boolean => {
	const compact = pathsListingFromEnvelope(payload);
	if (!compact) return false;
	ingestExploreListPrefix(compact);
	return true;
};

export const applyClosedEnvelope = (payload: unknown): boolean => {
	const compact = closedListingFromEnvelope(payload);
	if (!compact) return false;
	ingestHistoryClosedBatch(compact.entries);
	seedFromClosedLots();
	return true;
};
