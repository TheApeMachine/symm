import * as THREE from "three";
import type { FluidFields, FluidGrid } from "./wire";

export const MAXIMUM_FLUID_GRID_AXIS = 128;
export const MAXIMUM_VOLUME_STEPS = Math.ceil(
	Math.sqrt(3) * MAXIMUM_FLUID_GRID_AXIS,
);

export type FluidFieldTextures = {
	grid: FluidGrid;
	density: THREE.Data3DTexture;
	momentum: THREE.Data3DTexture;
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
	// The native arrays have Z varying fastest. Giving Three the dimensions in
	// Z,Y,X order preserves those bytes; shaders sample with coordinate.zyx.
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

const finite = (value: number, name: string, index: number) => {
	if (!Number.isFinite(value)) {
		throw new Error(`${name}[${index}] is not finite`);
	}

	return value;
};

const inverseMaximum = (maximum: number) => (maximum > 0 ? 1 / maximum : 0);

export const createFluidFieldTextures = (
	fields: FluidFields,
): FluidFieldTextures => {
	const { Grid: grid } = fields;

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

	const density = new Float32Array(fields.Density.length);
	const momentum = new Float32Array(fields.Momentum.length);
	const internalEnergy = new Float32Array(fields.InternalEnergy.length);
	const waveReal = new Float32Array(fields.WaveReal.length);
	const waveImaginary = new Float32Array(fields.WaveImaginary.length);
	let maximumDensity = 0;
	let maximumMomentum = 0;
	let maximumEnergy = 0;
	let maximumWaveMagnitude = 0;

	for (let cell = 0; cell < density.length; cell += 1) {
		density[cell] = finite(fields.Density[cell], "Density", cell);
		internalEnergy[cell] = finite(
			fields.InternalEnergy[cell],
			"InternalEnergy",
			cell,
		);
		waveReal[cell] = finite(fields.WaveReal[cell], "WaveReal", cell);
		waveImaginary[cell] = finite(
			fields.WaveImaginary[cell],
			"WaveImaginary",
			cell,
		);
		const momentumOffset = cell * 3;
		const momentumX = finite(
			fields.Momentum[momentumOffset],
			"Momentum",
			momentumOffset,
		);
		const momentumY = finite(
			fields.Momentum[momentumOffset + 1],
			"Momentum",
			momentumOffset + 1,
		);
		const momentumZ = finite(
			fields.Momentum[momentumOffset + 2],
			"Momentum",
			momentumOffset + 2,
		);
		momentum[momentumOffset] = momentumX;
		momentum[momentumOffset + 1] = momentumY;
		momentum[momentumOffset + 2] = momentumZ;
		maximumDensity = Math.max(maximumDensity, Math.abs(density[cell]));
		maximumEnergy = Math.max(maximumEnergy, Math.abs(internalEnergy[cell]));
		maximumMomentum = Math.max(
			maximumMomentum,
			Math.hypot(momentumX, momentumY, momentumZ),
		);
		maximumWaveMagnitude = Math.max(
			maximumWaveMagnitude,
			waveReal[cell] * waveReal[cell] +
				waveImaginary[cell] * waveImaginary[cell],
		);
	}

	const textures = {
		density: texture(density, grid, THREE.RedFormat),
		momentum: texture(momentum, grid, THREE.RGBFormat),
		internalEnergy: texture(internalEnergy, grid, THREE.RedFormat),
		waveReal: texture(waveReal, grid, THREE.RedFormat),
		waveImaginary: texture(waveImaginary, grid, THREE.RedFormat),
	};

	return {
		grid,
		...textures,
		densityScale: inverseMaximum(maximumDensity),
		momentumScale: inverseMaximum(maximumMomentum),
		energyScale: inverseMaximum(maximumEnergy),
		waveScale: inverseMaximum(maximumWaveMagnitude),
		dispose: () => {
			for (const value of Object.values(textures)) {
				value.dispose();
			}
		},
	};
};
