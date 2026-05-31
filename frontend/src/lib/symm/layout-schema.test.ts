import { describe, expect, it } from "vitest";

import {
	defaultLayoutDocument,
	isLayoutDocument,
} from "#/lib/symm/layout-schema";
import {
	createStreamDataProvider,
	readHeightMatrix,
} from "#/lib/symm/stream-data-provider";

describe("layout schema", () => {
	it("accepts backend layout payloads", () => {
		const layout = defaultLayoutDocument();
		expect(isLayoutDocument(layout)).toBe(true);
		expect(layout.panels.some((panel) => panel.type === "surface")).toBe(true);
	});

	it("reads nested height matrices", () => {
		const matrix = readHeightMatrix(
			{
				grid: {
					heights: [
						[1, 2],
						[3, 4],
					],
				},
			},
			"grid.heights",
		);

		expect(matrix).toEqual([
			[1, 2],
			[3, 4],
		]);
	});
});

describe("stream data provider", () => {
	it("routes stream updates to subscribers", () => {
		const provider = createStreamDataProvider();
		const seen: unknown[] = [];

		const unregister = provider.subscribe("field_snapshot", (payload) => {
			seen.push(payload);
		});

		provider.ingest("field_snapshot", { event: "field_snapshot", grid: {} });
		unregister();

		expect(seen).toHaveLength(1);
	});
});
