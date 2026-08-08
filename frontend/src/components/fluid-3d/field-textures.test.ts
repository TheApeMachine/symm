import { describe, expect, it } from "vitest";
import { createFluidFieldTextures } from "./field-textures";
import type { FluidFields } from "./wire";

const fields = (): FluidFields => ({
	Grid: { x: 2, y: 2, z: 2, spacing: 0.5 },
	Density: [0, 1, 0, 0, 0, 0, 0, 0],
	Momentum: [
		3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	],
	InternalEnergy: [0, 2, 0, 0, 0, 0, 0, 0],
	WaveReal: [0, 2, 0, 0, 0, 0, 0, 0],
	WaveImaginary: [0, 0, 0, 0, 0, 0, 0, 0],
});

describe("createFluidFieldTextures", () => {
	it("preserves native Z-fastest bytes and derives field scales", () => {
		const result = createFluidFieldTextures(fields());

		expect(result.density.image).toMatchObject({
			width: 2,
			height: 2,
			depth: 2,
		});
		expect(result.density.image.data).toEqual(
			new Float32Array([0, 1, 0, 0, 0, 0, 0, 0]),
		);
		expect(result.densityScale).toBe(1);
		expect(result.momentumScale).toBe(0.2);
		expect(result.energyScale).toBe(0.5);
		expect(result.waveScale).toBe(0.25);
		result.dispose();
	});

	it("rejects a non-finite field value", () => {
		const invalid = fields();
		invalid.WaveReal[3] = Number.NaN;

		expect(() => createFluidFieldTextures(invalid)).toThrow(
			"WaveReal[3] is not finite",
		);
	});
});
