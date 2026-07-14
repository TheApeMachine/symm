import {
	createSnapshotRowStore,
	hypothesisRowKey,
} from "#/collections/snapshot-retain";
import type { ThesisHypothesis } from "#/types/thesis";

const asHypotheses = (frame: unknown): ThesisHypothesis[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is ThesisHypothesis =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as ThesisHypothesis).symbol === "string" &&
			typeof (row as ThesisHypothesis).claim === "string",
	);
};

/*
hypothesesStore merges backend thesis.Hypotheses snapshots by row identity so
partial tick frames cannot erase retained symbol evidence.
*/
export const hypothesesStore = createSnapshotRowStore(
	"hypotheses",
	asHypotheses,
	hypothesisRowKey,
);
