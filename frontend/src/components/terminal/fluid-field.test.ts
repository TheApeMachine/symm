import { describe, expect, it } from "vitest";
import {
	buildGuidanceFlowLattice,
	buildPilotFlowLattice,
	compositeFieldLattice,
	isFluidFieldMatrix,
	normalizeFlowLattice,
	normalizeFluidLattice,
	sampleBilinearLattice,
	terminalFluidFieldStats,
} from "./fluid-field";

const matrix = [
	[0, 0.2, 0.4],
	[0.1, 0.9, 0.3],
	[0, 0.5, 1],
];

describe("isFluidFieldMatrix", () => {
	it("requires a two-dimensional lattice", () => {
		expect(isFluidFieldMatrix(matrix)).toBe(true);
		expect(isFluidFieldMatrix([[0.6, 0.4, 0.1]])).toBe(false);
		expect(isFluidFieldMatrix([])).toBe(false);
	});
});

describe("sampleBilinearLattice", () => {
	it("interpolates between neighboring lattice cells", () => {
		const lattice = [
			[0, 1],
			[0, 1],
		];

		expect(sampleBilinearLattice(lattice, 0, 0)).toBe(0);
		expect(sampleBilinearLattice(lattice, 1, 0.5)).toBe(1);
		expect(sampleBilinearLattice(lattice, 0.5, 0.5)).toBe(0.5);
	});
});

describe("normalizeFluidLattice", () => {
	it("expands sparse rho deposits using median/MAD contrast", () => {
		const sparse = [
			[0, 0, 0, 0],
			[0, 0.01, 0.02, 0],
			[0, 0, 0, 0],
		];

		expect(normalizeFluidLattice(sparse, false).peak).toBeGreaterThan(0.5);
	});
});

describe("buildPilotFlowLattice", () => {
	it("splats oscillator velocities onto the rho lattice", () => {
		const lattice = [
			[0, 0, 0],
			[0, 0, 0],
			[0, 0, 0],
		];
		const flow = buildPilotFlowLattice(lattice, [
			{
				source: "manifold",
				role: "whale_carrier",
				cellX: 1,
				cellY: 0,
				cellZ: 1,
				phase: 0,
				omega: 1,
				amplitude: 2,
				heat: 1,
				velX: 0.4,
				velY: 0,
				velZ: -0.2,
				speed: 0.447,
			},
		]);

		expect(flow.flowX[1][1]).toBeCloseTo(0.4);
		expect(flow.flowY[1][1]).toBeCloseTo(-0.2);
	});
});

describe("buildGuidanceFlowLattice", () => {
	it("maps projected guidance velocities directly onto the lattice", () => {
		const velX = [
			[0.1, 0.2],
			[0.3, 0.4],
		];
		const velZ = [
			[-0.1, -0.2],
			[-0.3, -0.4],
		];
		const flow = buildGuidanceFlowLattice(velX, velZ);

		expect(flow.flowX).toEqual(velX);
		expect(flow.flowY).toEqual(velZ);
	});
});

describe("compositeFieldLattice", () => {
	it("blends rho and pilot-wave magnitude into one display lattice", () => {
		const rho = [
			[0, 0.2],
			[0.1, 0.3],
		];
		const psiMag2 = [
			[0.5, 0.1],
			[0, 0.4],
		];

		expect(compositeFieldLattice(rho, psiMag2)).toEqual([
			[0.425, 0.2],
			[0.1, 0.34],
		]);
	});
});

describe("normalizeFlowLattice", () => {
	it("rescales guidance velocities by their median speed", () => {
		const flow = normalizeFlowLattice({
			flowX: [
				[0, 0],
				[0, 1.5],
			],
			flowY: [
				[0, 0],
				[0, 2],
			],
		});

		expect(flow.flowX[1][1]).toBeCloseTo(0.6);
		expect(flow.flowY[1][1]).toBeCloseTo(0.8);
	});
});

describe("terminalFluidFieldStats", () => {
	it("reports grid size, peak, and mad-based outliers", () => {
		expect(terminalFluidFieldStats(matrix)).toMatchObject({
			columns: 3,
			rows: 3,
			outliers: 2,
		});
		expect(terminalFluidFieldStats(matrix).peak).toBeGreaterThan(0.8);
	});

	it("returns zeroed stats for empty matrices", () => {
		expect(terminalFluidFieldStats([])).toEqual({
			columns: 0,
			rows: 0,
			peak: 0,
			outliers: 0,
		});
	});

	it("applies contour quantization in contour mode", () => {
		const contourMatrix = [
			[0.11, 0.23],
			[0.35, 0.47],
		];

		expect(terminalFluidFieldStats(contourMatrix, true)).toMatchObject({
			columns: 2,
			rows: 2,
			peak: 0.96,
		});
	});
});
