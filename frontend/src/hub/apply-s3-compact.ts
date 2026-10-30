import { ingestExploreListPrefix } from "#/collections/cognition";
import type { S3PrefixCompact } from "#/hub/s3-prefix-compact";
import { ingestHistoryClosedBatch } from "#/lib/lifecycle";

export const applyS3PrefixCompacts = (compacts: S3PrefixCompact[]) => {
	for (const compact of compacts) {
		if (compact.kind === "closed") {
			ingestHistoryClosedBatch(compact.entries);
			continue;
		}

		ingestExploreListPrefix(compact);
	}
};
