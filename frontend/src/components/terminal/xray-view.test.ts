import { describe, expect, it } from "vitest";
import {
	hawkesMetricsFromBuffer,
	hawkesMetricsFromFrames,
	intensitySeriesFromRingRows,
	latentPointsFromFrames,
	xrayLayersFromResonance,
} from "#/components/terminal/xray-view";
import { resonanceUniverseRows } from "#/lib/resonance";
import { Value } from "#/types/value";

const hawkesValue = (
	at: string,
	metric: string,
	side: string,
	raw: number,
): Value => {
	const key = side === "" ? metric : `${metric}:${side}`;
	const ts = BigInt(Number(at) || 0) * 1_000_000_000n;
	const prefix = "hawkes/signal/BTC-USD";
	return new Value({
		origin: "hawkes",
		role: "signal",
		scope: "BTC/USD",
		prefix,
		timestamp: ts,
		index: [`${prefix}/${key}`],
		layers: [new TextEncoder().encode(JSON.stringify([raw]))],
	});
};

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

	it("leaves error_norm null when state or prediction are absent or lengths mismatch, never inventing error from surprise", () => {
		const layers = xrayLayersFromResonance({
			source: "resonance",
			symbol: "BTC/USD",
			at: "2026-07-12T00:00:00Z",
			surprise: 0.85,
			layers: [
				{ state: [0.1, -0.2], prediction: [0.0] },
				{ state: [], prediction: [] },
				{ state: [0.5] },
			],
		});

		expect(layers).toHaveLength(3);
		expect(layers[0]?.error_norm).toBeNull();
		expect(layers[1]?.error_norm).toBeNull();
		expect(layers[2]?.error_norm).toBeNull();
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
			hawkesValue("1", "arrival_rate", "buy", 0.15),
			hawkesValue("1", "arrival_rate", "sell", 0.1),
			hawkesValue("2", "conditional_intensity", "buy", 0.3),
			hawkesValue("2", "conditional_intensity", "sell", 0.2),
			hawkesValue("2", "background_rate", "buy", 0.05),
			hawkesValue("2", "background_rate", "sell", 0.04),
			hawkesValue("2", "spectral_radius", "", 0.8),
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
			hawkesValue("1", "conditional_intensity", "buy", 0.3),
			hawkesValue("1", "conditional_intensity", "sell", 0.2),
			hawkesValue("1", "background_rate", "buy", 0.05),
			hawkesValue("1", "background_rate", "sell", 0.04),
			hawkesValue("1", "spectral_radius", "", 0.8),
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

	it("unfolds flat wire keys into hierarchy layers and latent points", () => {
		const universe = resonanceUniverseRows({
			"BTC/USD": {
				energy: 0.12,
				surprise: 0.45,
				latent_0: 0.2,
				latent_1: -0.4,
				layer_0_state_0: 1,
				layer_0_prediction_0: 0.5,
				layer_1_state_0: 0.3,
				layer_1_prediction_0: 0.1,
			},
			"ETH/USD": {
				latent_0: -0.1,
				latent_1: 0.7,
			},
		});

		const btc = universe.find((row) => row.symbol === "BTC/USD");
		const layers = xrayLayersFromResonance(btc);

		expect(layers).toHaveLength(2);
		expect(layers[0]?.state).toEqual([1]);
		expect(layers[0]?.prediction).toEqual([0.5]);
		expect(latentPointsFromFrames(universe)).toEqual([
			{
				key: "BTC/USD:",
				symbol: "BTC/USD",
				x: 0.2,
				y: -0.4,
				category: "",
			},
			{
				key: "ETH/USD:",
				symbol: "ETH/USD",
				x: -0.1,
				y: 0.7,
				category: "",
			},
		]);
	});
});