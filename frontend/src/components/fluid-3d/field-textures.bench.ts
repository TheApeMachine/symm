import { bench, describe } from "vitest";
import { packTexture3D } from "./field-textures";
import type { FluidFields } from "./wire";

const axis = 64;
const cells = axis * axis * axis;
const fields: FluidFields = {
	sequence: 1n,
	grid: { x: axis, y: axis, z: axis, spacing: 1 / axis },
	momRho: new Float32Array(cells * 4),
	internalEnergy: new Float32Array(cells),
	waveReal: new Float32Array(cells),
	waveImaginary: new Float32Array(cells),
	densityScale: 1,
	momentumScale: 1,
	energyScale: 1,
	waveScale: 1,
};

describe("packTexture3D", () => {
	bench("keeps one 64³ resident field slab without a transpose copy", () => {
		packTexture3D(fields.momRho, fields.grid, 4);
		packTexture3D(fields.internalEnergy, fields.grid, 1);
		packTexture3D(fields.waveReal, fields.grid, 1);
		packTexture3D(fields.waveImaginary, fields.grid, 1);
	});
});
