import { describe, expect, it } from "vitest";
import type { Measurement } from "#/collections/measurements";
import {
	hawkesMetricsFromFrames,
	hawkesSamplesFromFrames,
	latentPointsFromFrames,
	xrayLayersFromManifold,
} from "./xray";

const hawkesMeasurement = (
	at: string,
	metric: string,
	side: string,
	raw: number,
): Measurement => ({
	source: "hawkes",
	metric,
	subject: "hawkes_process",
	stream: "trades",
	symbol: "BTC/USD",
	side,
	at,
	observedFrom: at,
	horizon: 0,
	unit: "dimensionless",
	raw,
	normalized: { value: 0, available: false },
	maturity: 1,
	uncertainty: { available: false },
	validity: { state: "provisional", readiness: "model" },
	scale: { kind: "observation_window", from: at, through: at },
});

describe("xray", () => {
	it("builds four hierarchy rows from manifold rho", () => {
		const layers = xrayLayersFromManifold(
			{
				rho: [
					[0, 1, 2, 3],
					[1, 2, 3, 4],
					[2, 3, 4, 5],
					[3, 4, 5, 6],
					[4, 5, 6, 7],
					[5, 6, 7, 8],
					[6, 7, 8, 9],
					[7, 8, 9, 10],
				],
			},
			null,
		);

		expect(layers).toHaveLength(4);
		expect(layers[0]?.label).toBe("L0 · sensory");
		expect(layers[3]?.label).toBe("L3 · macro");
		expect(layers.every((layer) => layer.state.length === 16)).toBe(true);
		expect(layers.every((layer) => layer.error_norm > 0)).toBe(true);
	});

	it("groups sequential hawkes metrics by observation epoch", () => {
		const frames = [
			hawkesMeasurement("1", "arrival_rate", "buy", 0.15),
			hawkesMeasurement("1", "arrival_rate", "sell", 0.1),
			hawkesMeasurement("1", "event_count", "", 12),
			hawkesMeasurement("2", "conditional_intensity", "buy", 0.3),
			hawkesMeasurement("2", "conditional_intensity", "sell", 0.2),
			hawkesMeasurement("2", "baseline_intensity", "buy", 0.05),
			hawkesMeasurement("2", "baseline_intensity", "sell", 0.04),
			hawkesMeasurement("2", "spectral_radius", "", 0.8),
			hawkesMeasurement("2", "event_count", "", 18),
		];
		const hawkes = hawkesSamplesFromFrames(frames, "BTC/USD");
		const metrics = hawkesMetricsFromFrames(frames);

		expect(hawkes.map((sample) => sample.intensity)).toEqual([0.25, 0.5]);
		expect(metrics).toMatchObject({
			intensity: 0.5,
			branching: 0.8,
			radius: 0.8,
			buyIntensity: 0.3,
			sellIntensity: 0.2,
			exo: 0.09,
		});
	});

	it("retains the legacy hawkes readout during migration", () => {
		const frame: Measurement = {
			...hawkesMeasurement("1", "conditional_intensity", "buy", 0),
			metric: undefined,
			metrics: { intensityRatio: 0.4 },
		};

		expect(hawkesSamplesFromFrames([frame], "BTC/USD")).toMatchObject([
			{ key: "1", symbol: "BTC/USD", intensity: 0.4 },
		]);
	});

	it("reads latent histories from live frame fields", () => {
		const latent = latentPointsFromFrames({
			"BTC/USD": {
				values: () => [
					{
						at: "2",
						category: "equilibrium",
						latent: [0.2, -0.4, 0.8],
					},
				],
			},
		});

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
