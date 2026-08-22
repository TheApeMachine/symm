import type { FluidFields, FluidGrid } from "./wire";

export const MAXIMUM_FLUID_GRID_AXIS = 128;
export const MAXIMUM_VOLUME_STEPS = Math.ceil(
	Math.sqrt(3) * MAXIMUM_FLUID_GRID_AXIS,
);
export const TEXTURE_BYTES_PER_ROW_ALIGNMENT = 256;

export type FluidTextureExtent = {
	width: number;
	height: number;
	depth: number;
};

export type PackedTexture3D = {
	data: Float32Array;
	bytesPerRow: number;
	rowsPerImage: number;
	extent: FluidTextureExtent;
};

export const fluidTextureExtent = (grid: FluidGrid): FluidTextureExtent => ({
	width: grid.x,
	height: grid.y,
	depth: grid.z,
});

export const alignedBytesPerRow = (width: number, bytesPerTexel: number) => {
	const unpadded = width * bytesPerTexel;
	return (
		Math.ceil(unpadded / TEXTURE_BYTES_PER_ROW_ALIGNMENT) *
		TEXTURE_BYTES_PER_ROW_ALIGNMENT
	);
};

export const validateGrid = (grid: FluidGrid) => {
	if (
		grid.x > MAXIMUM_FLUID_GRID_AXIS ||
		grid.y > MAXIMUM_FLUID_GRID_AXIS ||
		grid.z > MAXIMUM_FLUID_GRID_AXIS
	) {
		throw new Error(
			`fluid grid exceeds the supported ${MAXIMUM_FLUID_GRID_AXIS}-cell axis`,
		);
	}

	if (grid.x <= 0 || grid.y <= 0 || grid.z <= 0) {
		throw new Error("fluid grid axes must be positive");
	}

	if (grid.spacing <= 0) {
		throw new Error("fluid grid spacing must be positive");
	}
};

/*
packTexture3D keeps Metal/Go X-fastest cell order (x + gx*(y + gy*z)) and only
inserts WebGPU row padding. Cubic 64³ uploads stay a view over the slab.
*/
export const packTexture3D = (
	source: Float32Array,
	grid: FluidGrid,
	channels: number,
): PackedTexture3D => {
	validateGrid(grid);
	const extent = fluidTextureExtent(grid);
	const expected = extent.width * extent.height * extent.depth * channels;

	if (source.length !== expected) {
		throw new Error("fluid texture source does not match the grid");
	}

	const bytesPerTexel = channels * Float32Array.BYTES_PER_ELEMENT;
	const bytesPerRow = alignedBytesPerRow(extent.width, bytesPerTexel);
	const rowFloats = extent.width * channels;
	const paddedRowFloats = bytesPerRow / Float32Array.BYTES_PER_ELEMENT;

	if (paddedRowFloats === rowFloats) {
		return {
			data: source,
			bytesPerRow,
			rowsPerImage: extent.height,
			extent,
		};
	}

	const packed = new Float32Array(
		paddedRowFloats * extent.height * extent.depth,
	);

	for (let slice = 0; slice < extent.depth; slice += 1) {
		for (let row = 0; row < extent.height; row += 1) {
			const sourceOffset = (slice * extent.height + row) * rowFloats;
			const destinationOffset = (slice * extent.height + row) * paddedRowFloats;
			packed.set(
				source.subarray(sourceOffset, sourceOffset + rowFloats),
				destinationOffset,
			);
		}
	}

	return {
		data: packed,
		bytesPerRow,
		rowsPerImage: extent.height,
		extent,
	};
};

export const packFluidFieldTextures = (fields: FluidFields) => ({
	momRho: packTexture3D(fields.momRho, fields.grid, 4),
	internalEnergy: packTexture3D(fields.internalEnergy, fields.grid, 1),
	waveReal: packTexture3D(fields.waveReal, fields.grid, 1),
	waveImaginary: packTexture3D(fields.waveImaginary, fields.grid, 1),
	densityScale: fields.densityScale,
	momentumScale: fields.momentumScale,
	energyScale: fields.energyScale,
	waveScale: fields.waveScale,
	grid: fields.grid,
});
