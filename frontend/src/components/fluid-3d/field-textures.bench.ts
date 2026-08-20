import { bench, describe } from "vitest";
import {
	createFluidFieldTextures,
	updateFluidFieldTextures,
} from "./field-textures";
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

describe("updateFluidFieldTextures", () => {
	const textures = createFluidFieldTextures(fields);

	bench("binds one 64³ resident field slab", () => {
		updateFluidFieldTextures(textures, fields);
	});
});
