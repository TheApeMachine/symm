import { describe, expect, it } from "vitest";
import { phaseCurrentGlyphs } from "./current";
import type { FluidFields } from "./wire";

const wave = (
	axis: number,
	winding: number,
	rotation = 0,
	amplitude = 1,
): FluidFields => {
	const dimensions = [8, 4, 16];
	const count = dimensions.reduce((product, size) => product * size, 1);
	const real = new Float32Array(count);
	const imaginary = new Float32Array(count);
	for (let cell = 0; cell < count; cell++) {
		const position = [
			Math.floor(cell / 64),
			Math.floor(cell / 16) % 4,
			cell % 16,
		];
		const phase =
			rotation + (2 * Math.PI * winding * position[axis]) / dimensions[axis];
		real[cell] = amplitude * Math.cos(phase);
		imaginary[cell] = amplitude * Math.sin(phase);
	}
	return {
		sequence: 1n,
		grid: { x: 8, y: 4, z: 16, spacing: 1 / 16 },
		momRho: new Float32Array(count * 4),
		internalEnergy: new Float32Array(count),
		waveReal: real,
		waveImaginary: imaginary,
		densityScale: 0,
		momentumScale: 0,
		energyScale: 0,
		waveScale: amplitude,
	};
};

describe("phaseCurrentGlyphs", () => {
	it("draws no current in vacuum or a spatially constant complex field", () => {
		expect(phaseCurrentGlyphs(wave(0, 0, 0, 0)).vertices).toHaveLength(0);
		expect(phaseCurrentGlyphs(wave(0, 0, 0.7)).vertices).toHaveLength(0);
	});
	it("preserves all three axes and signed periodic flux on a noncubic Z-fastest grid", () => {
		for (const axis of [0, 1, 2]) {
			for (const winding of [-1, 1]) {
				const fields = wave(axis, winding);
				const { vertices, peak } = phaseCurrentGlyphs(fields);
				const size = [fields.grid.x, fields.grid.y, fields.grid.z][axis];
				expect(peak).toBeCloseTo(
					Math.abs(Math.sin((2 * Math.PI) / size)) / fields.grid.spacing,
					5,
				);
				expect(vertices.length).toBe(8 * 4 * 16 * 30);
				for (let glyph = 0; glyph < vertices.length; glyph += 30) {
					for (const direction of [0, 1, 2]) {
						const delta =
							vertices[glyph + 5 + direction] - vertices[glyph + direction];
						if (direction === axis) {
							expect(Math.sign(delta)).toBe(winding);
							continue;
						}
						expect(delta).toBeCloseTo(0, 7);
					}
				}
			}
		}
	});
	it("is invariant to global phase and keeps amplitude in the reported flux scale", () => {
		const reference = phaseCurrentGlyphs(wave(2, 1));
		const rotated = phaseCurrentGlyphs(wave(2, 1, 0.6));
		const scaled = phaseCurrentGlyphs(wave(2, 1, 0, 3));
		expect(rotated.peak).toBeCloseTo(reference.peak, 5);
		expect(scaled.peak).toBeCloseTo(9 * reference.peak, 4);
		for (let index = 0; index < reference.vertices.length; index++) {
			expect(rotated.vertices[index]).toBeCloseTo(reference.vertices[index], 5);
			expect(scaled.vertices[index]).toBeCloseTo(reference.vertices[index], 5);
		}
	});
});
