import { beforeEach, describe, expect, it } from "vitest";
import {
	latestManifoldParticles,
	latestManifoldWave,
	paintManifoldParticles,
	paintManifoldWave,
} from "#/providers/manifold-parts";

describe("manifold-parts", () => {
	beforeEach(() => {
		paintManifoldWave(null);
		paintManifoldParticles(null);
	});

	it("retains object packets for wave and particles keyed by symbol", () => {
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

		expect(latestManifoldWave("BTC/USD")?.wave).toHaveLength(1);
		expect(latestManifoldParticles("BTC/USD")?.particles).toHaveLength(1);
	});

	it("accepts a single-row array without clearing the payload", () => {
		paintManifoldWave([
			{
				source: "manifold",
				symbol: "BTC/USD",
				wave: [{ omega: 2, real: 0.3, imaginary: 0, linewidth: 0.1 }],
			},
		]);

		expect(latestManifoldWave("BTC/USD")?.symbol).toBe("BTC/USD");
		expect(latestManifoldWave("BTC/USD")?.wave).toHaveLength(1);
	});

	it("retains multi-symbol array payloads independently", () => {
		paintManifoldWave([
			{
				source: "manifold",
				symbol: "BTC/USD",
				wave: [{ omega: 1, real: 0.2, imaginary: 0.1, linewidth: 0.1 }],
			},
			{
				source: "manifold",
				symbol: "ETH/USD",
				wave: [{ omega: 3, real: 0.5, imaginary: 0.2, linewidth: 0.1 }],
			},
		]);

		expect(latestManifoldWave("BTC/USD")?.wave).toHaveLength(1);
		expect(latestManifoldWave("ETH/USD")?.wave).toHaveLength(1);
		expect(latestManifoldWave("BTC/USD")?.wave?.[0]).toMatchObject({
			omega: 1,
		});
		expect(latestManifoldWave("ETH/USD")?.wave?.[0]).toMatchObject({
			omega: 3,
		});
	});

	it("returns null when no symbol-specific payload exists", () => {
		paintManifoldWave({
			source: "manifold",
			symbol: "BTC/USD",
			wave: [{ omega: 1, real: 0.2, imaginary: 0.1, linewidth: 0.1 }],
		});

		expect(latestManifoldWave("ETH/USD")).toBeNull();
	});

	it("clears when the wire value is empty", () => {
		paintManifoldWave({ source: "manifold", wave: [{ omega: 1 }] });
		paintManifoldWave([]);
		expect(latestManifoldWave("")).toBeNull();

		paintManifoldParticles({ source: "manifold", particles: [{}] });
		paintManifoldParticles(null);
		expect(latestManifoldParticles("")).toBeNull();
	});
});
