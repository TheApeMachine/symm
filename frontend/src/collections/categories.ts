import {
	categoryRowKey,
	createSnapshotRowStore,
} from "#/collections/snapshot-retain";
import type { ThesisCategory } from "#/types/thesis";

const asCategories = (frame: unknown): ThesisCategory[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is ThesisCategory =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as ThesisCategory).type === "string",
	);
};

/*
categoriesStore merges backend thesis.Categories snapshots by row identity so
partial tick frames cannot erase retained symbol evidence.
*/
export const categoriesStore = createSnapshotRowStore(
	"categories",
	asCategories,
	categoryRowKey,
);
