import { describe, expect, it } from "vitest";

import {
	predictionVisibleXRange,
	predictionWindowMin,
} from "#/components/charts/prediction/prediction-chart-state";

describe("predictionWindowMin", () => {
	it("keeps two horizons of history behind the latest forecast target", () => {
		const rightEdge = 1_710_000_120;
		const horizonSec = 60;

		expect(predictionWindowMin(rightEdge, horizonSec)).toBe(
			rightEdge - 2 * horizonSec,
		);
	});
});

describe("predictionVisibleXRange", () => {
	it("anchors the right edge to the latest forecast point", () => {
		const nowSec = 1_710_000_000;
		const horizonSec = 60;

		expect(
			predictionVisibleXRange(horizonSec, 1_710_000_090, null, null, nowSec),
		).toEqual({
			minX: 1_709_999_970,
			maxX: 1_710_000_090,
		});
	});

	it("expands left to keep settled ground truth in view", () => {
		const nowSec = 1_710_000_000;
		const horizonSec = 60;

		expect(
			predictionVisibleXRange(
				horizonSec,
				1_710_000_090,
				1_709_999_850,
				1_709_999_860,
				nowSec,
			),
		).toEqual({
			minX: 1_709_999_850,
			maxX: 1_710_000_090,
		});
	});

	it("caps the visible span to four horizons", () => {
		const nowSec = 1_710_000_000;
		const horizonSec = 60;

		expect(
			predictionVisibleXRange(
				horizonSec,
				1_710_000_090,
				1_709_998_000,
				null,
				nowSec,
			),
		).toEqual({
			minX: 1_709_999_850,
			maxX: 1_710_000_090,
		});
	});
});

describe("predictionVisibleXRange validation", () => {
	it("rejects non-positive horizonSec", () => {
		expect(() =>
			predictionVisibleXRange(0, 1_710_000_090, null, null, 1_710_000_000),
		).toThrow(RangeError);
	});
});

describe("predictionVisibleXRange validation", () => {
	it("rejects non-positive horizonSec", () => {
		expect(() =>
			predictionVisibleXRange(0, 1_710_000_090, null, null, 1_710_000_000),
		).toThrow(RangeError);
	});
});
