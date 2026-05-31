import { beforeEach, describe, expect, it } from "vitest";

import {
	ConfidenceDataProvider,
	isConfidenceRow,
} from "#/components/symm/confidence-data-provider";

describe("isConfidenceRow", () => {
	it("accepts hub confidence payloads", () => {
		expect(
			isConfidenceRow({
				source: "pumpdump",
				confidence: 0.42,
				count: 3,
			}),
		).toBe(true);
	});

	it("rejects ohlc rows", () => {
		expect(
			isConfidenceRow({
				symbol: "BTC/EUR",
				open: 1,
				high: 2,
				low: 0.5,
				close: 1.5,
			}),
		).toBe(false);
	});
});

describe("ConfidenceDataProvider.ingest", () => {
	beforeEach(() => {
		ConfidenceDataProvider.reset();
	});

	it("routes mean confidence to registered sources", () => {
		const received: number[] = [];
		const unregister = ConfidenceDataProvider.registerSource(
			"hawkes",
			(row) => {
				received.push(row.confidence);
			},
		);

		ConfidenceDataProvider.ingest({
			source: "hawkes",
			confidence: 0.35,
			count: 2,
		});

		expect(received).toEqual([0.35]);
		expect(ConfidenceDataProvider.snapshot().get("hawkes")?.confidence).toBe(
			0.35,
		);
		unregister();
	});

	it("replays latest confidence on register", () => {
		ConfidenceDataProvider.ingest({
			source: "causal",
			confidence: 0.12,
			count: 1,
		});

		const received: number[] = [];
		const unregister = ConfidenceDataProvider.registerSource(
			"causal",
			(row) => {
				received.push(row.confidence);
			},
		);

		expect(received).toEqual([0.12]);
		unregister();
	});

	it("routes backend aliases to the visible gauge source", () => {
		const received: number[] = [];
		const unregister = ConfidenceDataProvider.registerSource(
			"liquidity",
			(row) => {
				received.push(row.confidence);
			},
		);

		ConfidenceDataProvider.ingest({
			source: "basis",
			confidence: 0.64,
		});

		expect(received).toEqual([0.64]);
		expect(ConfidenceDataProvider.snapshot().get("liquidity")?.confidence).toBe(
			0.64,
		);
		unregister();
	});

	it("hydrates gauges from wallet snapshot confidence", () => {
		const received: number[] = [];
		const unregister = ConfidenceDataProvider.registerSource(
			"hawkes",
			(row) => {
				received.push(row.confidence);
			},
		);

		ConfidenceDataProvider.ingestSnapshot({
			hawkes: 0.51,
			causal: "stale",
		});

		expect(received).toEqual([0.51]);
		expect(ConfidenceDataProvider.snapshot().get("hawkes")?.confidence).toBe(
			0.51,
		);
		expect(ConfidenceDataProvider.snapshot().has("causal")).toBe(false);
		unregister();
	});

	it("preserves gauge sub-metrics for tooltip rendering", () => {
		const received: string[] = [];
		const unregister = ConfidenceDataProvider.registerSource("fluid", (row) => {
			received.push(row.factors?.map((factor) => factor.name).join(",") ?? "");
		});

		ConfidenceDataProvider.ingest({
			source: "fluid",
			confidence: 1.8,
			factors: [
				{ name: "div", value: 0.2 },
				{ name: "re", value: 1.1 },
			],
		});

		expect(received).toEqual(["div,re"]);
		expect(ConfidenceDataProvider.snapshot().get("fluid")?.factors).toEqual([
			{ name: "div", value: 0.2 },
			{ name: "re", value: 1.1 },
		]);
		unregister();
	});
});
