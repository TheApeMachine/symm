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
			(confidence) => {
				received.push(confidence);
			},
		);

		ConfidenceDataProvider.ingest({
			source: "hawkes",
			confidence: 0.35,
			count: 2,
		});

		expect(received).toEqual([0.35]);
		expect(ConfidenceDataProvider.snapshot().get("hawkes")).toBe(0.35);
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
			(confidence) => {
				received.push(confidence);
			},
		);

		expect(received).toEqual([0.12]);
		unregister();
	});

	it("routes backend aliases to the visible gauge source", () => {
		const received: number[] = [];
		const unregister = ConfidenceDataProvider.registerSource(
			"liquidity",
			(confidence) => {
				received.push(confidence);
			},
		);

		ConfidenceDataProvider.ingest({
			source: "basis",
			confidence: 0.64,
		});

		expect(received).toEqual([0.64]);
		expect(ConfidenceDataProvider.snapshot().get("liquidity")).toBe(0.64);
		unregister();
	});
});
