import { bench, describe } from "vitest";
import { createFluidFieldTextures } from "./field-textures";
import type { FluidFields } from "./wire";

const axis = 64;
const cells = axis * axis * axis;
const fields: FluidFields = {
	Grid: { x: axis, y: axis, z: axis, spacing: 1 / axis },
	Density: Array.from({ length: cells }, (_, index) => index / cells),
	Momentum: Array.from({ length: cells * 3 }, (_, index) => index / cells),
	InternalEnergy: Array.from({ length: cells }, (_, index) => index / cells),
	WaveReal: Array.from({ length: cells }, (_, index) => index / cells),
	WaveImaginary: Array.from({ length: cells }, (_, index) => index / cells),
};

describe("createFluidFieldTextures", () => {
	bench("uploads one 64³ resident field set", () => {
		createFluidFieldTextures(fields).dispose();
	});
});
