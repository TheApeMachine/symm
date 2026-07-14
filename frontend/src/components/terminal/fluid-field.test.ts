import { describe, expect, it } from "vitest";
import {
	isFluidFieldMatrix,
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

describe("terminalFluidFieldStats", () => {
	it("reports grid size, peak, and mad-based outliers", () => {
		expect(terminalFluidFieldStats(matrix)).toEqual({
			columns: 3,
			rows: 3,
			peak: 1,
			outliers: 2,
		});
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
