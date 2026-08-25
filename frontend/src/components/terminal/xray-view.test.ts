import { describe, expect, it } from "vitest";
import type { Measurement } from "#/collections/types";
import {
	hawkesMetricsFromBuffer,
	hawkesMetricsFromFrames,
	latentPointsFromFrames,
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
});
