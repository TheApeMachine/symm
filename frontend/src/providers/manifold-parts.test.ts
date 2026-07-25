import { describe, expect, it } from "vitest";
import {
	latestManifoldParticles,
	latestManifoldWave,
	paintManifoldParticles,
	paintManifoldWave,
} from "#/providers/manifold-parts";

describe("manifold-parts", () => {
	it("retains object packets for wave and particles", () => {
		paintManifoldWave({
			source: "manifold",
			symbol: "BTC/USD",
			wave: [{ omega: 1, real: 0.2, imaginary: 0.1, linewidth: 0.1 }],
		});
		paintManifoldParticles({
			source: "manifold",
			symbol: "BTC/USD",
			particles: [{ cell_x: 1, cell_z: 2 }],
		});

		expect(latestManifoldWave()?.wave).toHaveLength(1);
		expect(latestManifoldParticles()?.particles).toHaveLength(1);
	});

	it("accepts a single-row array without clearing the payload", () => {
		paintManifoldWave([
			{
				source: "manifold",
				symbol: "BTC/USD",
				wave: [{ omega: 2, real: 0.3, imaginary: 0, linewidth: 0.1 }],
			},
		]);

		expect(latestManifoldWave()?.symbol).toBe("BTC/USD");
		expect(latestManifoldWave()?.wave).toHaveLength(1);
	});

	it("clears when the wire value is empty", () => {
		paintManifoldWave({ source: "manifold", wave: [{ omega: 1 }] });
		paintManifoldWave([]);
		expect(latestManifoldWave()).toBeNull();

		paintManifoldParticles({ source: "manifold", particles: [{}] });
		paintManifoldParticles(null);
		expect(latestManifoldParticles()).toBeNull();
	});
});
