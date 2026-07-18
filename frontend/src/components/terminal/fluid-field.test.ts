import { describe, expect, it } from "vitest";
import {
	blendGasIntoPilot,
	buildGuidanceFlowLattice,
	buildPilotFlowLattice,
	isFluidFieldMatrix,
	meanGuidanceSpeed,
	normalizeFlowLattice,
	normalizeFluidLattice,
	resampleFluidLattice,
	resolvePilotDisplayLattice,
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
	it("keeps uniform positive lattices lit instead of collapsing to zero", () => {
		const constant = [
			[0.4, 0.4, 0.4],
			[0.4, 0.4, 0.4],
			[0.4, 0.4, 0.4],
		];
		const { normalized, peak } = normalizeFluidLattice(constant, false);

		expect(peak).toBeGreaterThan(0);
		expect(normalized.flat().every((value) => value > 0)).toBe(true);
	});

	it("expands sparse rho deposits using median/MAD contrast", () => {
		const sparse = [
			[0, 0, 0, 0],
			[0, 0.01, 0.02, 0],
			[0, 0, 0, 0],
		];

		expect(normalizeFluidLattice(sparse, false).peak).toBeGreaterThan(0.5);
	});

	it("maps non-finite cells to zero before peak tracking", () => {
		const lattice = [
			[Number.NaN, 0.5],
			[0.25, Number.POSITIVE_INFINITY],
		];
		const { normalized, peak } = normalizeFluidLattice(lattice, false);

		expect(Number.isFinite(peak)).toBe(true);
		expect(normalized[0][0]).toBe(0);
		expect(normalized[1][1]).toBe(0);
	});
});

describe("resampleFluidLattice", () => {
	it("projects mismatched guidance velocities onto the rho grid", () => {
		const source = [
			[0, 1],
			[0, 1],
		];

		expect(resampleFluidLattice(source, 3, 3)).toEqual([
			[0, 0.5, 1],
			[0, 0.5, 1],
			[0, 0.5, 1],
		]);
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
				role: "particle",
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

describe("meanGuidanceSpeed", () => {
	it("averages Bohm current magnitude across the lattice", () => {
		const velX = [
			[3, 0],
			[0, 0],
		];
		const velZ = [
			[4, 0],
			[0, 0],
		];

		expect(meanGuidanceSpeed(velX, velZ)).toBeCloseTo(1.25);
	});
});

describe("resolvePilotDisplayLattice", () => {
	it("prefers |ψ|² over gas ρ for the primary cloud", () => {
		const rho = [
			[0, 0.2],
			[0.1, 0.3],
		];
		const psiMag2 = [
			[0.5, 0.1],
			[0, 0.4],
		];

		expect(resolvePilotDisplayLattice(rho, psiMag2)).toEqual(psiMag2);
	});

	it("falls back to gas ρ when |ψ|² is absent", () => {
		const rho = [
			[0, 0.2],
			[0.1, 0.3],
		];

		expect(resolvePilotDisplayLattice(rho)).toEqual(rho);
	});
});

describe("blendGasIntoPilot", () => {
	it("lets sparse ρ fill where |ψ|² is thin without owning bright cells", () => {
		const psiMag2 = [
			[0.5, 0.1],
			[0, 0.4],
		];
		const rho = [
			[0, 0.8],
			[0.6, 0.1],
		];

		const blended = blendGasIntoPilot(psiMag2, rho);

		expect(blended[0][0]).toBeCloseTo(0.5);
		expect(blended[0][1]).toBeCloseTo(0.36);
		expect(blended[1][0]).toBeCloseTo(0.27);
		expect(blended[1][1]).toBeCloseTo(0.4);
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
