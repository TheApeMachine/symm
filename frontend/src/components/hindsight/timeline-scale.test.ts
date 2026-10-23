import { describe, expect, it } from "vitest";
import type {
	HindsightTimeline,
	HindsightTimelineBucket,
} from "./hindsight-types";
import { buildScale } from "./timeline-scale";

const bucket = (
	index: number,
	from: number,
	to: number,
): HindsightTimelineBucket => ({
	index,
	fromSequence: from,
	toSequence: to,
	fromAt: "2026-10-17T00:22:35Z",
	toAt: "2026-10-17T00:22:35Z",
	observedFromSequence: from,
	observedToSequence: to,
	observedFromAt: "2026-10-17T00:22:35Z",
	observedToAt: "2026-10-17T00:22:35Z",
	observations: 1,
	tickers: 0,
	trades: 1,
	tradeQty: 1,
	defined: true,
	open: 1,
	high: 1,
	low: 1,
	close: 1,
	spreadFraction: 0,
	hasSpreadFraction: false,
	touchDepth: 0,
	hasTouchDepth: false,
	captureRate: 0,
	hasCaptureRate: false,
});

const timeline = (buckets: HindsightTimelineBucket[]): HindsightTimeline => ({
	run: "1",
	symbol: "BTC/USD",
	coordinate: "midpoint",
	axis: "time",
	span: { fromSequence: 1, toSequence: 6, fromAt: "", toAt: "" },
	runSpan: { fromSequence: 1, toSequence: 6, fromAt: "", toAt: "" },
	buckets,
	discovery: {
		symbol: "BTC/USD",
		coordinate: "midpoint",
		stored: 0,
		episodes: [],
		insufficientData: false,
	},
	streams: [],
	symbols: [],
});

describe("buildScale", () => {
	it("puts a click on the slot that was drawn, not the next stored bucket", () => {
		const scale = buildScale(timeline([bucket(0, 1, 2), bucket(2, 5, 6)]), 300);

		expect(scale.xOf(5)).toBeCloseTo(200);
		expect(scale.bucketAt(210)?.index).toBe(2);
		expect(scale.bucketAt(20)?.index).toBe(0);
		expect(scale.sequenceAt(210)).toBe(5);
		expect(scale.xOf(scale.sequenceAt(210))).toBeCloseTo(200);
	});
});
