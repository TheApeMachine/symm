import * as THREE from "three";
import type { FluidFields, FluidGrid } from "./wire";

export const MAXIMUM_FLUID_GRID_AXIS = 128;
export const MAXIMUM_VOLUME_STEPS = Math.ceil(
	Math.sqrt(3) * MAXIMUM_FLUID_GRID_AXIS,
);

export type FluidFieldTextures = {
	grid: FluidGrid;
	momRho: THREE.Data3DTexture;
	internalEnergy: THREE.Data3DTexture;
	waveReal: THREE.Data3DTexture;
	waveImaginary: THREE.Data3DTexture;
	densityScale: number;
	momentumScale: number;
	energyScale: number;
	waveScale: number;
	dispose: () => void;
};

const texture = (
	values: Float32Array,
	grid: FluidGrid,
	format: THREE.PixelFormat,
) => {
	// Metal and the wire both keep Z varying fastest. Three receives Z,Y,X so
	// shaders can sample coordinate.zyx without transposing the volume.
	const result = new THREE.Data3DTexture(values, grid.z, grid.y, grid.x);
	result.format = format;
	result.type = THREE.FloatType;
	result.minFilter = THREE.LinearFilter;
	result.magFilter = THREE.LinearFilter;
	result.generateMipmaps = false;
	result.unpackAlignment = 1;
	result.needsUpdate = true;
	return result;
};

const validateGrid = (grid: FluidGrid) => {
	if (
		grid.x > MAXIMUM_FLUID_GRID_AXIS ||
		grid.y > MAXIMUM_FLUID_GRID_AXIS ||
		grid.z > MAXIMUM_FLUID_GRID_AXIS
	) {
		throw new Error(
			`fluid grid exceeds the supported ${MAXIMUM_FLUID_GRID_AXIS}-cell axis`,
		);
	}

	if (grid.spacing <= 0) {
		throw new Error("fluid grid spacing must be positive");
	}
};

export const createFluidFieldTextures = (
	fields: FluidFields,
): FluidFieldTextures => {
	validateGrid(fields.grid);
	const textures = {
		momRho: texture(fields.momRho, fields.grid, THREE.RGBAFormat),
		internalEnergy: texture(
			fields.internalEnergy,
			fields.grid,
			THREE.RedFormat,
		),
		waveReal: texture(fields.waveReal, fields.grid, THREE.RedFormat),
		waveImaginary: texture(fields.waveImaginary, fields.grid, THREE.RedFormat),
	};

	return {
		grid: fields.grid,
		...textures,
		densityScale: fields.densityScale,
		momentumScale: fields.momentumScale,
		energyScale: fields.energyScale,
		waveScale: fields.waveScale,
		dispose: () => {
			for (const value of Object.values(textures)) {
				value.dispose();
			}
		},
	};
};

export const updateFluidFieldTextures = (
	textures: FluidFieldTextures,
	fields: FluidFields,
) => {
	validateGrid(fields.grid);

	if (
		textures.grid.x !== fields.grid.x ||
		textures.grid.y !== fields.grid.y ||
		textures.grid.z !== fields.grid.z
	) {
		throw new Error("fluid grid dimensions changed during a resident session");
	}

	textures.momRho.image.data = fields.momRho;
	textures.internalEnergy.image.data = fields.internalEnergy;
	textures.waveReal.image.data = fields.waveReal;
	textures.waveImaginary.image.data = fields.waveImaginary;
	textures.momRho.needsUpdate = true;
	textures.internalEnergy.needsUpdate = true;
	textures.waveReal.needsUpdate = true;
	textures.waveImaginary.needsUpdate = true;
	textures.grid = fields.grid;
	textures.densityScale = fields.densityScale;
	textures.momentumScale = fields.momentumScale;
	textures.energyScale = fields.energyScale;
	textures.waveScale = fields.waveScale;
};
