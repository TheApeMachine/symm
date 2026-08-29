import { beforeEach, describe, expect, it } from "vitest";
import type { Measurement } from "#/collections/types";
import {
	clearRetainedTelemetry,
	getAllRetainedResonance,
	getRetainedCognition,
	getRetainedHawkes,
	getRetainedResonance,
	hawkesMetricsFromBuffer,
	hawkesMetricsFromFrames,
	intensitySeriesFromRingRows,
	latentPointsFromFrames,
	retainCognitionRow,
	retainHawkesMetric,
	retainResonanceRow,
	xrayLayersFromResonance,
} from "#/components/terminal/xray-view";

const hawkesMeasurement = (
	at: string,
	metric: string,
	side: string,
	raw: number,
): Measurement => ({
	source: "hawkes",
	symbol: "BTC/USD",
	at,
	observedFrom: at,
	horizon: 0,
	maturity: 1,
	uncertainty: null,
	metrics: {
		[side === "" ? metric : `${metric}:${side}`]: {
			raw,
			unit: "dimensionless",
		},
	},
});

describe("xray-view", () => {
	beforeEach(() => {
		clearRetainedTelemetry();
	});

	it("builds hierarchy rows from resonance layers", () => {
		const layers = xrayLayersFromResonance({
			source: "resonance",
			symbol: "BTC/USD",
			at: "2026-07-12T00:00:00Z",
			surprise: 0.25,
			layers: [
				{ state: [0.1, -0.2, 0.3], prediction: [0.0, -0.1, 0.2] },
				{ state: [0.4], prediction: [0.5] },
			],
		});

		expect(layers).toHaveLength(2);
		expect(layers[0]?.label).toBe("L0 · sensory");
		expect(layers[1]?.label).toBe("L1 · macro");
		expect(layers[0]?.state).toEqual([0.1, -0.2, 0.3]);
		expect(layers[0]?.prediction).toEqual([0.0, -0.1, 0.2]);
		expect(layers[0]?.error_norm).toBeCloseTo(0.1, 5);
	});

	it("ignores manifold density when building hierarchy", () => {
		expect(
			xrayLayersFromResonance({
				source: "resonance",
				symbol: "BTC/USD",
				at: "2026-07-12T00:00:00Z",
				rho: [
					[0, 1],
					[1, 0],
				],
			}),
		).toEqual([]);
	});

	it("groups sequential hawkes metrics by observation epoch", () => {
		const frames = [
			hawkesMeasurement("1", "arrival_rate", "buy", 0.15),
			hawkesMeasurement("1", "arrival_rate", "sell", 0.1),
			hawkesMeasurement("2", "conditional_intensity", "buy", 0.3),
			hawkesMeasurement("2", "conditional_intensity", "sell", 0.2),
			hawkesMeasurement("2", "background_rate", "buy", 0.05),
			hawkesMeasurement("2", "background_rate", "sell", 0.04),
			hawkesMeasurement("2", "spectral_radius", "", 0.8),
		];
		const metrics = hawkesMetricsFromFrames(frames);

		expect(metrics).toMatchObject({
			intensity: 0.5,
			branching: 0.8,
			radius: 0.8,
			buyIntensity: 0.3,
			sellIntensity: 0.2,
			exo: 0.09,
		});
	});

	it("reads hawkes metrics from retained observation epochs", () => {
		const frames = [
			hawkesMeasurement("1", "conditional_intensity", "buy", 0.3),
			hawkesMeasurement("1", "conditional_intensity", "sell", 0.2),
			hawkesMeasurement("1", "background_rate", "buy", 0.05),
			hawkesMeasurement("1", "background_rate", "sell", 0.04),
			hawkesMeasurement("1", "spectral_radius", "", 0.8),
		];

		expect(hawkesMetricsFromBuffer(frames)).toMatchObject({
			intensity: 0.5,
			branching: 0.8,
		});
	});

	it("collapses ring rows into one intensity sample per epoch, sorted by arrival", () => {
		const series = intensitySeriesFromRingRows([
			{ at: 3n, raw: 0.4 },
			{ at: 1n, raw: 0.15 },
			{ at: 2n, raw: 0.3 },
			{ at: 1n, raw: 0.2 },
		]);

		expect(series).toEqual([0.2, 0.3, 0.4]);
	});

	it("never fabricates a zero for an epoch that reported no intensity", () => {
		// A ring buffer holds one row per emission, not per epoch, and most
		// emissions carry an unrelated Hawkes binding rather than intensity.
		// Only epochs present in the input contribute a sample.
		const series = intensitySeriesFromRingRows([
			{ at: 1n, raw: 0.5 },
			{ at: 4n, raw: 0.6 },
		]);

		expect(series).toEqual([0.5, 0.6]);
		expect(series).not.toContain(0);
	});

	it("reads latent histories from live resonance frames", () => {
		const latent = latentPointsFromFrames([
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: "2",
				category: "equilibrium",
				latent: [0.2, -0.4, 0.8],
			},
		]);

		expect(latent).toEqual([
			{
				key: "BTC/USD:2",
				symbol: "BTC/USD",
				x: 0.2,
				y: -0.4,
				category: "equilibrium",
			},
		]);
	});

	it("keeps only symbols with full latent pairs in each incoming batch", () => {
		const latent = latentPointsFromFrames([
			{
				source: "resonance",
				symbol: "BTC/USD",
				at: "2",
				category: "equilibrium",
				latent: [0.2, -0.4],
			},
			{
				source: "resonance",
				symbol: "ETH/USD",
				at: "3",
				category: "transition",
			},
		]);

		expect(latent).toEqual([
			{
				key: "BTC/USD:2",
				symbol: "BTC/USD",
				x: 0.2,
				y: -0.4,
				category: "equilibrium",
			},
		]);
	});

	it("retains per-symbol resonance state across sparse updates", () => {
		retainResonanceRow("BTC/USD", {
			symbol: "BTC/USD",
			energy: 0.42,
			layers: [{ state: [0.1], prediction: [0.0], errorNorm: 0.1 }],
		});

		retainResonanceRow("ETH/USD", {
			symbol: "ETH/USD",
			energy: 0.88,
		});

		const btc = getRetainedResonance("BTC/USD");
		expect(btc?.energy).toBe(0.42);
		expect(btc?.layers).toHaveLength(1);

		const all = getAllRetainedResonance();
		expect(all.length).toBeGreaterThanOrEqual(2);
	});

	it("retains per-symbol cognition and hawkes metrics", () => {
		retainCognitionRow("BTC/USD", {
			winner: "trend-up",
			confidence: 0.95,
		});

		expect(getRetainedCognition("BTC/USD")).toMatchObject({
			winner: "trend-up",
			confidence: 0.95,
		});

		retainHawkesMetric("BTC/USD", "conditional_intensity:buy", 0.1234);
		retainHawkesMetric("BTC/USD", "branching_spectral_radius", 0.75);

		const hwk = getRetainedHawkes("BTC/USD");
		expect(hwk["conditional_intensity:buy"]).toBe(0.1234);
		expect(hwk.branching_spectral_radius).toBe(0.75);
	});
});
