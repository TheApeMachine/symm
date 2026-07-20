import { describe, expect, it } from "vitest";
import {
	terminalFluidDisplayLatticeFromFrame,
	terminalFluidMatrixFromFrame,
	terminalFluidParticlesFromFrame,
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalWaveModesFromFrame,
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

describe("terminal phase dial frame", () => {
	it("preserves complex modes and signed scanner responses", () => {
		const frame = {
			phaseReady: true,
			phaseReason: "",
			wave: [
				{ omega: -1, real: 0.5, imaginary: -0.25, linewidth: 0.1 },
				{ omega: 1, real: -0.5, imaginary: 0.25, linewidth: 0.2 },
			],
			phaseScan: [
				{ angle: 0, similarity: 0.8, observedAt: "2026-07-20T12:00:00Z" },
				{
					angle: Math.PI,
					similarity: -0.8,
					observedAt: "2026-07-20T12:00:00Z",
				},
			],
		};

		expect(terminalWaveModesFromFrame(frame)).toEqual(frame.wave);
		expect(terminalPhaseScanFromFrame(frame)).toEqual(frame.phaseScan);
		expect(terminalPhaseStatusFromFrame(frame)).toEqual({
			ready: true,
			reason: "",
		});
	});

	it("rejects incomplete modes and scan rows", () => {
		expect(
			terminalWaveModesFromFrame({ wave: [{ real: 1, imaginary: 0 }] }),
		).toEqual([]);
		expect(terminalPhaseScanFromFrame({ phaseScan: [{ angle: 0 }] })).toEqual(
			[],
		);
		expect(
			terminalPhaseStatusFromFrame({
				phaseReady: false,
				phaseReason: "awaiting a prior nonzero phase observation",
			}),
		).toEqual({
			ready: false,
			reason: "awaiting a prior nonzero phase observation",
		});
	});
});
