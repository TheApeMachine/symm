import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { ResonanceFrame } from "#/collections/types";
import { updatePredictionHistory } from "#/components/charts/prediction";
import {
	predictiveCodingSeries,
	reconstructionError,
} from "#/components/charts/prediction/series";

const frame = (at: string, surprise: number): ResonanceFrame => ({
	source: "resonance",
	symbol: "BTC/USD",
	at,
	surprise,
	layers: [{ state: [surprise], prediction: [surprise / 2] }],
});

describe("updatePredictionHistory", () => {
	it("appends distinct observation epochs", () => {
		const history = Circular<ResonanceFrame>(3);

		updatePredictionHistory(history, [
			frame("2026-07-20T18:00:00Z", 1),
			frame("2026-07-20T18:00:01Z", 2),
		]);

		expect(history.values().map((entry) => entry.surprise)).toEqual([1, 2]);
	});

	it("updates a repeated current epoch instead of duplicating it", () => {
		const history = Circular<ResonanceFrame>(3);
		const at = "2026-07-20T18:00:00Z";

		updatePredictionHistory(history, [frame(at, 1)]);
		updatePredictionHistory(history, [frame(at, 3)]);

		expect(history.values()).toHaveLength(1);
		expect(history.values()[0]?.surprise).toBe(3);
	});

	it("keeps only the newest samples that fit the drawable capacity", () => {
		const history = Circular<ResonanceFrame>(2);

		updatePredictionHistory(history, [
			frame("2026-07-20T18:00:00Z", 1),
			frame("2026-07-20T18:00:01Z", 2),
			frame("2026-07-20T18:00:02Z", 3),
		]);

		expect(history.values().map((entry) => entry.surprise)).toEqual([2, 3]);
	});
});

describe("predictiveCodingSeries", () => {
	it("keeps all three layers and separates adjacent reconstruction from top context", () => {
		const frames: ResonanceFrame[] = [
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: "2026-07-20T18:00:00Z",
				layers: [
					{ state: [3, 4], prediction: [0, 0] },
					{ state: [1, 2], prediction: [1, 0] },
					{ state: [5, 12], prediction: [0, 0] },
				],
				expectedReturn: 0.012,
				uncertainty: 0.004,
				incrementalMSE: 0.0002,
				incrementalSkillLowerBound: 0.0001,
				calibrationSamples: 21,
				returnReady: true,
			},
		];

		const series = predictiveCodingSeries(frames);

		expect(series.layers).toHaveLength(3);
		expect(series.layers.map((layer) => layer.kind)).toEqual([
			"reconstruction",
			"reconstruction",
			"context",
		]);
		expect(series.layers.map((layer) => layer.values)).toEqual([
			[5],
			[2],
			[13],
		]);
		expect(series.layers[2]?.prediction).toBeNull();
		expect(series.returnHead).toMatchObject({
			expected: [0.012],
			upper: [0.016],
			lower: [0.008],
			ready: true,
			samples: 21,
		});
	});

	it("preserves malformed or missing epochs as explicit gaps", () => {
		expect(reconstructionError([1, 2], [1])).toBeNull();
		expect(reconstructionError([1, Number.NaN], [1, 2])).toBeNull();

		const series = predictiveCodingSeries([
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: "1",
				layers: [
					{ state: [1], prediction: [0] },
					{ state: [1], prediction: [0] },
				],
				expectedReturn: 0.01,
			},
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: "2",
				layers: [
					{ state: [1, 2], prediction: [1] },
					{ state: [2], prediction: [0] },
				],
			},
		]);

		expect(series.layers[0]?.values).toEqual([1, null]);
		expect(series.returnHead.expected).toEqual([0.01, null]);
		expect(series.returnHead.upper).toEqual([null, null]);
	});
});
