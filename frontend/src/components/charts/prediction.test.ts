import { describe, expect, it } from "vitest";
import type { ResonanceFrame } from "#/collections/types";
import {
	mergeResonanceData,
	parseResonanceData,
	parseResonanceFromWorkspace,
	signedVectorTransform,
	vectorSlotTransform,
} from "#/components/charts/prediction";
import {
	predictiveCodingSeries,
	reconstructionError,
} from "#/components/charts/prediction/series";
import { Value } from "#/types/value";

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
					probeHorizon: 2,
					posterior: [{ Scale: 0.004 }],
				},
				taskRelativePrecision: 0.75,
				taskRelativePrecisionReady: true,
				forecastValidity: { state: "valid", readiness: "forecast" },
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
			taskRelativePrecision: 0.75,
			horizon: 1,
			reach: 2,
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
					probeHorizon: 1,
				},
				taskRelativePrecision: 0.5,
				taskRelativePrecisionReady: false,
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

describe("signedVectorTransform", () => {
	it("uses the same upward-positive convention for state and prediction bars", () => {
		expect(signedVectorTransform).toBe("scaleY(calc(var(--value, 0) * -1))");
	});
});

const wireResonanceReading = (
	metrics: Record<string, number>,
): Value => {
	const prefix = "resonance/reading/BTC-USD/0000000000000000000/0000000000000000100";
	const encoder = new TextEncoder();
	const index: string[] = [];
	const layers: Uint8Array[] = [];

	for (const [name, raw] of Object.entries(metrics)) {
		index.push(`${prefix}/${name}`);
		layers.push(encoder.encode(JSON.stringify([raw])));
	}

	return new Value({
		origin: "resonance",
		role: "reading",
		scope: "BTC-USD",
		prefix,
		index,
		layers,
	});
};

describe("mergeResonanceData", () => {
	it("replaces hierarchy layers on every workspace patch", () => {
		const prior = {
			forwardCurve: [0.1],
			layers: [{ state: [1], prediction: [0] }],
		};
		const patch = {
			forwardCurve: [0.2],
			layers: [{ state: [9], prediction: [3] }],
		};

		const merged = mergeResonanceData(prior, patch, "workspace");
		expect(merged.layers?.[0]?.state).toEqual([9]);
		expect(merged.layers?.[0]?.prediction).toEqual([3]);
	});
});

describe("parseResonanceData", () => {
	it("densifies sparse latent and forward_curve metrics into lane vectors", () => {
		const val = wireResonanceReading({
			latent_0: 0.2,
			latent_2: -0.4,
			forward_curve_0: 0.01,
			forward_curve_1: -0.02,
			layer_0_state_0: 1,
			layer_0_prediction_0: 0.5,
		});

		const parsed = parseResonanceData(val);
		expect(parsed?.latent).toEqual([0.2, 0, -0.4]);
		expect(parsed?.forwardCurve).toEqual([0.01, -0.02]);
		expect(parsed?.layers?.[0]?.state).toEqual([1]);
		expect(parsed?.layers?.[0]?.prediction).toEqual([0.5]);
	});

	it("reads calibrated and supported_horizon from numeric wire metrics", () => {
		const parsed = parseResonanceData(
			wireResonanceReading({
				calibrated: 1,
				supported_horizon: 2,
			}),
		);
		expect(parsed?.calibrated).toBe(true);
		expect(parsed?.supportedHorizon).toBe(2);
	});

	it("treats energy and surprise as resonance content on the aggregate tick", () => {
		const prefix = "workspace/tick/aggregate/1/100";
		const frame = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			index: [
				`${prefix}/resonance/BTC-USD/energy`,
				`${prefix}/resonance/BTC-USD/surprise`,
			],
			layers: [
				new TextEncoder().encode(JSON.stringify([1.2])),
				new TextEncoder().encode(JSON.stringify([0.05])),
			],
		});

		const parsed = parseResonanceFromWorkspace("BTC/USD", frame);
		expect(parsed?.energy).toBe(1.2);
		expect(parsed?.surprise).toBe(0.05);
	});

	it("folds resonance layers from the workspace aggregate tick", () => {
		const prefix = "workspace/tick/aggregate/1/100";
		const frame = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			index: [
				`${prefix}/resonance/BTC-USD/layer_0_state_0`,
				`${prefix}/resonance/BTC-USD/layer_0_prediction_0`,
				`${prefix}/resonance/BTC-USD/energy`,
			],
			layers: [
				new TextEncoder().encode(JSON.stringify([0.9])),
				new TextEncoder().encode(JSON.stringify([0.4])),
				new TextEncoder().encode(JSON.stringify([1.2])),
			],
		});

		const parsed = parseResonanceFromWorkspace("BTC/USD", frame);
		expect(parsed?.layers?.[0]?.state).toEqual([0.9]);
		expect(parsed?.layers?.[0]?.prediction).toEqual([0.4]);
		expect(parsed?.energy).toBe(1.2);
	});
});
