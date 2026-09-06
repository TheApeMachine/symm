import { describe, expect, it } from "vitest";
import {
	alignedBytesPerRow,
	fluidTextureExtent,
	packTexture3D,
 packFluidFieldTextures,
	TEXTURE_BYTES_PER_ROW_ALIGNMENT,
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

describe("fluidTextureExtent", () => {
	it("binds Z-fastest Metal storage as WebGPU width, height, depth", () => {
		expect(fluidTextureExtent({ x: 4, y: 8, z: 16, spacing: 1 })).toEqual({
			width: 16,
			height: 8,
			depth: 4,
		});
	});
});

describe("packTexture3D", () => {
	it("binds an already-aligned slab without copying its values", () => {
		const grid = { x: 2, y: 2, z: 16, spacing: 1 / 16 };
		const source = new Float32Array(16 * 2 * 2 * 4);
		source[3] = 9;
		const packed = packTexture3D(source, grid, 4);

		expect(packed.extent).toEqual({ width: 16, height: 2, depth: 2 });
		expect(packed.bytesPerRow).toBe(TEXTURE_BYTES_PER_ROW_ALIGNMENT);
		expect(packed.data).toBe(source);
		expect(packed.data[3]).toBe(9);
	});

	it("pads short rows to the WebGPU 256-byte alignment without transposing axes", () => {
		const frame = fields();
		frame.internalEnergy[2] = 7;
		const packed = packTexture3D(frame.internalEnergy, frame.grid, 1);
		const paddedRowFloats =
			alignedBytesPerRow(2, Float32Array.BYTES_PER_ELEMENT) /
			Float32Array.BYTES_PER_ELEMENT;

		expect(packed.extent).toEqual({ width: 2, height: 2, depth: 2 });
		expect(packed.bytesPerRow).toBe(TEXTURE_BYTES_PER_ROW_ALIGNMENT);
		expect(packed.data).not.toBe(frame.internalEnergy);
		expect(packed.data[paddedRowFloats]).toBe(7);
	});
});

describe("packFluidFieldTextures", () => {
 it("normalizes small and large field peaks to the same displayed magnitude", () => {
  for (const peak of [0.000001, 0.25, 1, 1000]) {
   const frame = fields();
   frame.densityScale = peak;
   frame.momentumScale = peak;
   frame.energyScale = peak;
   frame.waveScale = peak;
   const packed = packFluidFieldTextures(frame);
   expect(peak * packed.densityScale).toBeCloseTo(1);
   expect(peak * packed.momentumScale).toBeCloseTo(1);
   expect(peak * packed.energyScale).toBeCloseTo(1);
   expect(Math.hypot(peak, peak) * packed.waveScale).toBeCloseTo(Math.SQRT2);
   expect(frame.waveScale).toBe(peak);
  }
 });
 it("keeps genuinely empty fields empty", () => {
  const frame = fields();
  frame.waveScale = 0;
  expect(packFluidFieldTextures(frame).waveScale).toBe(0);
 });
});
