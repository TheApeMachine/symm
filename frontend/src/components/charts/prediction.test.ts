import { describe, expect, it } from "vitest";
import type { ResonanceFrame } from "#/collections/types";
import { vectorSlotTransform } from "#/components/charts/prediction";
import {
	predictiveCodingSeries,
	reconstructionError,
} from "#/components/charts/prediction/series";

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
				forecast: {
					forwardCurve: [0.012],
					forwardRetention: [1],
					supportedHorizon: 1,
					expectedReturn: 0.012,
					expectedBasisPoints: 120,
					confidence: 0.75,
					confidenceReady: true,
					predictiveScale: 0.004,
					predictiveScaleBasisPoints: 40,
					degreesOfFreedom: 21,
				},
				forecastValidity: { state: "valid", readiness: "forecast" },
				uncertainty: 0.004,
				incrementalMSE: 0.0002,
				incrementalSkillLowerBound: 0.0001,
				calibrationSamples: 21,
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
			confidence: 0.75,
			horizon: 1,
			ready: true,
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
				forecast: {
					forwardCurve: [0.01],
					forwardRetention: [1],
					supportedHorizon: 1,
					expectedReturn: 0.01,
					expectedBasisPoints: 100,
					confidence: 0.5,
					confidenceReady: false,
					predictiveScale: 0,
					predictiveScaleBasisPoints: 0,
					degreesOfFreedom: 0,
				},
				forecastValidity: { state: "valid", readiness: "forecast" },
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

describe("vectorSlotTransform", () => {
	it("positions slots with transforms derived from the current vector length", () => {
		expect(vectorSlotTransform(0, 4)).toBe("translateX(0%) scaleX(0.25)");
		expect(vectorSlotTransform(2, 4)).toBe("translateX(50%) scaleX(0.25)");
		expect(vectorSlotTransform(2, 5)).toBe("translateX(40%) scaleX(0.2)");
	});
});
