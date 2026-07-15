import { createStore } from "@tanstack/react-store";
import type { ThesisCategory } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

const CATEGORY_HISTORY_LIMIT = 50;

const categoryKey = (row: ThesisCategory): string =>
	`${row.symbol ?? ""}:${row.type}`;

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
categoryValues returns the newest retained category row for each identity.
*/
export const categoryValues = (
	categories: Record<string, CircularBuffer<ThesisCategory>>,
): ThesisCategory[] =>
	Object.keys(categories)
		.sort()
		.flatMap((key) => {
			const category = categories[key]?.values().at(-1);

			return category === undefined ? [] : [category];
		});

/*
categoriesStore retains backend thesis categories in bounded circular buffers
so partial tick frames cannot erase retained symbol evidence.
*/
export const categoriesStore = createStore(
	{
		categories: {} as Record<string, CircularBuffer<ThesisCategory>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const rows = asCategories(frame);

				if (rows.length === 0) {
					return prev;
				}

				const categories = prev.categories;

				for (const row of rows) {
					const key = categoryKey(row);

					if (!categories[key]) {
						categories[key] = Circular<ThesisCategory>(CATEGORY_HISTORY_LIMIT);
					}

					categories[key].push(row);
				}

				return {
					categories,
					version: prev.version + 1,
				};
			}),
		reset: () =>
			setState(() => ({
				categories: {},
				version: 0,
			})),
	}),
);
