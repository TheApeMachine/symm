import { describe, expect, it } from "vitest";
import {
	hawkesSamplesFromFrames,
	latentPointsFromFrames,
	xrayLayersFromManifold,
} from "./xray";

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

	it("reads hawkes and latent histories from live frame fields", () => {
		const hawkes = hawkesSamplesFromFrames(
			[
				{ at: "1", metrics: { intensityRatio: 0.25 } },
				{ at: "2", metrics: { intensityRatio: 0.5 } },
			],
			"BTC/USD",
		);
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

		expect(hawkes.map((sample) => sample.intensity)).toEqual([0.25, 0.5]);
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
