import { describe, expect, it } from "vitest";
import {
	isDegenerateHeightmap,
	projectManifoldHeightmap,
} from "#/components/charts/manifold/manifold-grid";
import type { ManifoldFieldSnapshot } from "#/components/charts/manifold/types";

const sampleFrame = (): ManifoldFieldSnapshot => ({
	type: "manifold",
	ts: "2024-01-01T00:00:00Z",
	grid: { x: 3, y: 1, z: 2, spacing: 1 },
	rho: [
		[0, 1, 2],
		[1, 3, 4],
	],
	reading: {
		pressure_grad_x: 0,
		pressure_grad_y: 0,
		pressure_grad_z: 0,
		pressure_grad_norm: 1,
		divergence: 0,
		coherence_mag2: 0.5,
		guidance_speed: 0.2,
		viscosity_proxy: 0.1,
	},
	carriers: [
		{
			role: "whale",
			symbol: "XBT/USD",
			x: 1,
			y: 0,
			z: 1,
			cell_x: 1,
			cell_y: 0,
			cell_z: 1,
			amplitude: 0.2,
			heat: 0.8,
			omega: 1,
			phase: 0,
			vel_x: 0.2,
			vel_y: 0,
			vel_z: 0,
		},
	],
});

describe("projectManifoldHeightmap", () => {
	it("normalizes rho into a surface and bumps whale carriers", () => {
		const projected = projectManifoldHeightmap(sampleFrame(), 0, 1);

		expect(projected.gridX).toBe(3);
		expect(projected.gridZ).toBe(2);
		expect(projected.heights[1][1]).toBeGreaterThan(projected.heights[0][0]);
	});
});

describe("isDegenerateHeightmap", () => {
	it("detects flat surfaces", () => {
		expect(isDegenerateHeightmap([[0.5, 0.5]])).toBe(true);
		expect(isDegenerateHeightmap([[0, 0.5, 1]])).toBe(false);
	});
});
