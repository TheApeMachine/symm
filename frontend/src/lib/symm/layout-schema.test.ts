import { describe, expect, it } from "vitest";

import {
	defaultLayoutDocument,
	gaugeGridLayout,
	gaugeSourcesFor,
	hasGaugeSources,
	isLayoutDocument,
	mergedGaugePanel,
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

	it("derives gauge sources from layout panels", () => {
		expect(
			gaugeSourcesFor({
				type: "gauge_grid",
				sources: ["fluid", "hawkes"],
			}),
		).toEqual(["fluid", "hawkes"]);
	});

	it("returns an empty gauge list until the backend manifest arrives", () => {
		expect(gaugeSourcesFor({ type: "gauge_grid" })).toEqual([]);
		expect(hasGaugeSources({ type: "gauge_grid" })).toBe(false);
		expect(hasGaugeSources({ type: "gauge_grid", sources: ["fluid"] })).toBe(
			true,
		);
	});

	it("sizes the gauge grid from source count", () => {
		expect(gaugeGridLayout(["a", "b", "c", "d", "e"])).toEqual({
			columns: 4,
			rows: 2,
		});
	});

	it("merges grid and strip panels for heatmaps", () => {
		const layout = defaultLayoutDocument();
		layout.panels = [
			{
				type: "gauge_grid",
				sources: ["hawkes", "fluid"],
				labels: { hawkes: "Hawkes", fluid: "Fluid" },
			},
			{
				type: "gauge_strip",
				sources: ["toxicity"],
				labels: { toxicity: "Toxic" },
			},
		];

		expect(mergedGaugePanel(layout)?.sources).toEqual([
			"hawkes",
			"fluid",
			"toxicity",
		]);
	});

	it("accepts layout payloads with anchor_symbol", () => {
		expect(
			isLayoutDocument({
				event: "layout",
				ts: "2026-06-02T00:00:00Z",
				anchor_symbol: "ETH/EUR",
				panels: [{ type: "audit_panel" }],
			}),
		).toBe(true);
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
