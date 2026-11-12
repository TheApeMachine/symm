import { signals } from "#/collections/app";
import { ingestExploreListPrefix } from "#/collections/cognition";
import { seedFromClosedLots } from "#/collections/longtail";
import {
	closedListingFromEnvelope,
	pathsListingFromEnvelope,
} from "#/hub/s3-prefix-compact";
import { ingestHistoryClosedBatch } from "#/lib/lifecycle";

/*
ListPrefix frames land on signals.pathbook / signals.closed; subscriptions
fold them into cognition and history. The websocket stays a key→store push.
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

signals.pathbook.subscribe(() => {
	applyPathsEnvelope(signals.pathbook.state);
});

signals.closed.subscribe(() => {
	applyClosedEnvelope(signals.closed.state);
});
