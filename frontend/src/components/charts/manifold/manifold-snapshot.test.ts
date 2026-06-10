import { describe, expect, it } from "vitest";
import {
	formatManifoldReading,
	parseManifoldSnapshot,
} from "#/components/charts/manifold/manifold-snapshot";

describe("parseManifoldSnapshot", () => {
	it("coerces reading metrics from wire payloads", () => {
		const frame = parseManifoldSnapshot({
			type: "manifold",
			ts: "2024-01-01T00:00:00Z",
			grid: { x: 32, y: 3, z: 16, spacing: 0.01 },
			rho: [[0.1, 0.2]],
			reading: {
				pressure_grad_norm: "0.0042",
				coherence_mag2: 0.15,
				guidance_speed: 0.02,
				viscosity_proxy: 3.5,
				divergence: -0.001,
			},
			carriers: [
				{
					role: "symbol",
					symbol: "BTC/USD",
					x: 1,
					y: 0,
					z: 2,
					cell_x: 1,
					cell_y: 0,
					cell_z: 2,
					amplitude: 0.4,
					heat: 0,
					omega: 1,
					phase: 0,
					vel_x: 0,
					vel_y: 0,
					vel_z: 0,
				},
			],
		});

		expect(frame).not.toBeNull();
		expect(frame?.reading.pressure_grad_norm).toBe(0.0042);
		expect(frame?.reading.coherence_mag2).toBe(0.15);
		expect(frame?.carriers).toHaveLength(1);
	});

	it("formats non-zero readings without collapsing to 0.00e+0", () => {
		const lines = formatManifoldReading({
			type: "manifold",
			ts: "",
			grid: { x: 32, y: 3, z: 16, spacing: 0.01 },
			rho: [[0.1]],
			reading: {
				pressure_grad_x: 0,
				pressure_grad_y: 0,
				pressure_grad_z: 0,
				pressure_grad_norm: 0.0042,
				divergence: -0.001,
				coherence_mag2: 0.15,
				guidance_speed: 0.02,
				viscosity_proxy: 3.5,
			},
			carriers: [
				{
					role: "symbol",
					symbol: "BTC/USD",
					x: 0,
					y: 0,
					z: 0,
					cell_x: 0,
					cell_y: 0,
					cell_z: 0,
					amplitude: 0.15,
					heat: 0,
					omega: 1,
					phase: 0,
					vel_x: 0,
					vel_y: 0,
					vel_z: 0,
				},
			],
		});

		expect(lines[0]).toContain("4.20e-3");
		expect(lines[1]).toContain("0.1500");
		expect(lines[2]).toContain("carrier guidance");
	});
});
