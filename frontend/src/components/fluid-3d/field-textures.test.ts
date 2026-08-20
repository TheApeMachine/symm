import { describe, expect, it } from "vitest";
import {
	createFluidFieldTextures,
	updateFluidFieldTextures,
} from "./field-textures";
import type { FluidFields } from "./wire";

const fields = (): FluidFields => ({
	sequence: 1n,
	grid: { x: 2, y: 2, z: 2, spacing: 0.5 },
	momRho: new Float32Array(8 * 4),
	internalEnergy: new Float32Array(8),
	waveReal: new Float32Array(8),
	waveImaginary: new Float32Array(8),
	densityScale: 1,
	momentumScale: 0.2,
	energyScale: 0.5,
	waveScale: 0.25,
});

describe("createFluidFieldTextures", () => {
	it("binds the received slab views without copying their values", () => {
		const frame = fields();
		const result = createFluidFieldTextures(frame);

		expect(result.momRho.image).toMatchObject({
			width: 2,
			height: 2,
			depth: 2,
		});
		expect(result.momRho.image.data).toBe(frame.momRho);
		expect(result.internalEnergy.image.data).toBe(frame.internalEnergy);
		expect(result.waveReal.image.data).toBe(frame.waveReal);
		expect(result.waveImaginary.image.data).toBe(frame.waveImaginary);
		expect(result.densityScale).toBe(1);
		expect(result.momentumScale).toBe(0.2);
		expect(result.energyScale).toBe(0.5);
		expect(result.waveScale).toBe(0.25);
		result.dispose();
	});

	it("reuses resident texture objects for the next same-grid slab", () => {
		const first = fields();
		const second = fields();
		second.sequence = 2n;
		const result = createFluidFieldTextures(first);

		updateFluidFieldTextures(result, second);

		expect(result.momRho.image.data).toBe(second.momRho);
		expect(result.internalEnergy.image.data).toBe(second.internalEnergy);
		expect(result.waveReal.image.data).toBe(second.waveReal);
		expect(result.waveImaginary.image.data).toBe(second.waveImaginary);
		result.dispose();
	});
});
