import { describe, expect, it } from "vitest";
import {
	terminalFluidMatrixFromFrame,
	terminalFluidParticlesFromFrame,
} from "./charts";

describe("terminalFluidParticlesFromFrame", () => {
	it("reads post-step manifold particles instead of legacy carriers", () => {
		const particles = terminalFluidParticlesFromFrame({
			particles: [
				{
					source: "fluid",
					role: "whale_carrier",
					cell_x: 7,
					cell_y: 2,
					cell_z: 5,
					phase: 0.4,
					omega: 1.7,
					amplitude: 3,
					heat: 4,
					vel_x: 0.1,
					vel_y: 0.2,
					vel_z: 0.3,
					speed: 0.374,
				},
			],
			carriers: [{ cell_x: 0, cell_z: 0 }],
		});

		expect(particles).toEqual([
			{
				source: "fluid",
				role: "whale_carrier",
				cellX: 7,
				cellY: 2,
				cellZ: 5,
				phase: 0.4,
				omega: 1.7,
				amplitude: 3,
				heat: 4,
				velX: 0.1,
				velY: 0.2,
				velZ: 0.3,
				speed: 0.374,
			},
		]);
	});

	it("rejects incomplete particle records", () => {
		const particles = terminalFluidParticlesFromFrame({
			particles: [{ cell_x: 7, cell_z: 5 }],
		});

		expect(particles).toEqual([]);
	});
});

describe("terminalFluidMatrixFromFrame", () => {
	it("renders the typed scalar manifold readout directly", () => {
		expect(
			terminalFluidMatrixFromFrame({
				bidTouchDensity: 0.6,
				askTouchDensity: 0.4,
				pressureGradX: 0.1,
				divergence: -0.2,
				coherenceMag2: 0.3,
			}),
		).toEqual([[0.6, 0.4, 0.1, -0.2, 0.3]]);
	});
});
