import { describe, expect, it } from "vitest";
import {
	terminalFluidDisplayLatticeFromFrame,
	terminalFluidMatrixFromFrame,
	terminalFluidParticlesFromFrame,
} from "./charts";

describe("terminalFluidParticlesFromFrame", () => {
	it("reads post-step manifold particles instead of legacy carriers", () => {
		const particles = terminalFluidParticlesFromFrame({
			particles: [
				{
					source: "fluid",
					role: "particle",
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
				role: "particle",
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

describe("terminalFluidDisplayLatticeFromFrame", () => {
	it("returns the gas-blended pilot lattice when both fields exist", () => {
		const psiMag2 = [
			[0.5, 0.1],
			[0, 0.4],
		];
		const rho = [
			[0, 0.2],
			[0.1, 0.3],
		];

		expect(
			terminalFluidDisplayLatticeFromFrame({
				rho,
				psiMag2,
			}),
		).toEqual([
			[0.5, 0.1],
			[0.045000000000000005, 0.4],
		]);
	});
});

describe("terminalFluidMatrixFromFrame", () => {
	it("reads nested manifold reading scalars", () => {
		expect(
			terminalFluidMatrixFromFrame({
				reading: {
					pressureGradX: 0.1,
					divergence: -0.2,
					coherenceMag2: 0.3,
				},
			}),
		).toEqual([[0.1, -0.2, 0.3]]);
	});

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
