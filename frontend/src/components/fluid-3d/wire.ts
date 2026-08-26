const SLAB_HEADER_BYTES = 64;
const SLAB_VERSION = 1;
const FIELD_MAGIC = "SFF1";
const PARTICLE_MAGIC = "SPF1";
const PHASE_MAGIC = "SPH1";
const PARTICLE_STRIDE_FLOATS = 12;

export type FluidGrid = {
	x: number;
	y: number;
	z: number;
	spacing: number;
};

export type FluidFields = {
	sequence: bigint;
	grid: FluidGrid;
	momRho: Float32Array;
	internalEnergy: Float32Array;
	waveReal: Float32Array;
	waveImaginary: Float32Array;
	densityScale: number;
	momentumScale: number;
	energyScale: number;
	waveScale: number;
};

export type FluidVector = {
	X: number;
	Y: number;
	Z: number;
};

export type FluidParticle = {
	Position: FluidVector;
	Velocity: FluidVector;
	Mass: number;
	Heat: number;
	Energy: number;
	Phase: number;
	Omega: number;
	Amplitude: number;
};

export class FluidParticleFrame {
	constructor(
		readonly sequence: bigint,
		readonly count: number,
		readonly stride: number,
		readonly heatScale: number,
		readonly energyScale: number,
		readonly massScale: number,
		readonly amplitudeScale: number,
		readonly values: Float32Array,
	) {}

	particle(index: number): FluidParticle | null {
		if (index < 0 || index >= this.count) {
			return null;
		}

		const offset = index * this.stride;

		return {
			Position: {
				X: this.values[offset + 0],
				Y: this.values[offset + 1],
				Z: this.values[offset + 2],
			},
			Velocity: {
				X: this.values[offset + 3],
				Y: this.values[offset + 4],
				Z: this.values[offset + 5],
			},
			Mass: this.values[offset + 6],
			Heat: this.values[offset + 7],
			Energy: this.values[offset + 8],
			Phase: this.values[offset + 9],
			Omega: this.values[offset + 10],
			Amplitude: this.values[offset + 11],
		};
	}
}

const header = (bytes: Uint8Array, magic: string) => {
	if (bytes.byteLength < SLAB_HEADER_BYTES) {
		throw new Error(`${magic} slab is truncated`);
	}

	const found = String.fromCharCode(...bytes.subarray(0, magic.length));

	if (found !== magic) {
		throw new Error(`expected ${magic} slab, received ${found}`);
	}

	const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);

	if (view.getUint16(4, true) !== SLAB_VERSION) {
		throw new Error(`${magic} slab version is unsupported`);
	}

	if (view.getUint16(6, true) !== SLAB_HEADER_BYTES) {
		throw new Error(`${magic} slab header size is invalid`);
	}

	return view;
};

const floatView = (bytes: Uint8Array, offset: number, length: number) =>
	new Float32Array(bytes.buffer, bytes.byteOffset + offset, length);

const requireFloatRange = (
	bytes: Uint8Array,
	offset: number,
	length: number,
	name: string,
) => {
	const byteLength = length * Float32Array.BYTES_PER_ELEMENT;

	if (
		offset < SLAB_HEADER_BYTES ||
		offset % Float32Array.BYTES_PER_ELEMENT !== 0 ||
		offset + byteLength > bytes.byteLength
	) {
		throw new Error(`${name} slab range is invalid`);
	}
};

/*
The phase slab carries the resident phase reading, the spectral mode lattice,
and a downsampled oscillator phase sample, all taken from the live sensorium
domain after one step.
*/
export type FluidPhaseReading = {
	divergence: number;
	guidanceSpeed: number;
	coherenceMag2: number;
	pressureGradNorm: number;
	viscosityProxy: number;
	kuramotoR: number;
};

export type FluidWaveMode = {
	omega: number;
	real: number;
	imaginary: number;
	linewidth: number;
};

export type FluidOscillator = {
	phase: number;
	omega: number;
};

export type FluidPhase = {
	sequence: bigint;
	reading: FluidPhaseReading;
	oscillators: FluidOscillator[];
	modes: FluidWaveMode[];
};

export const decodePhase = (bytes: Uint8Array): FluidPhase => {
	const view = header(bytes, PHASE_MAGIC);
	const sequence = view.getBigUint64(8, true);
	const reading = {
		divergence: view.getFloat32(16, true),
		guidanceSpeed: view.getFloat32(20, true),
		coherenceMag2: view.getFloat32(24, true),
		pressureGradNorm: view.getFloat32(28, true),
		viscosityProxy: view.getFloat32(32, true),
		kuramotoR: view.getFloat32(36, true),
	};
	const oscillatorCount = view.getUint32(40, true);
	const modeCount = view.getUint32(44, true);
	const byteLength = view.getUint32(48, true);

	if (byteLength !== bytes.byteLength) {
		throw new Error("SPH1 slab byte length does not match its header");
	}

	const phases = floatView(bytes, 64, oscillatorCount);
	const omegas = floatView(bytes, 64 + oscillatorCount * 4, oscillatorCount);
	const modesOffset = 64 + oscillatorCount * 8;
	const modeOmega = floatView(bytes, modesOffset, modeCount);
	const modeReal = floatView(bytes, modesOffset + modeCount * 4, modeCount);
	const modeImag = floatView(
		bytes,
		modesOffset + modeCount * 8,
		modeCount,
	);
	const modeLinewidth = floatView(
		bytes,
		modesOffset + modeCount * 12,
		modeCount,
	);

	const oscillators: FluidOscillator[] = [];

	for (let index = 0; index < oscillatorCount; index++) {
		oscillators.push({ phase: phases[index], omega: omegas[index] });
	}

	const modes: FluidWaveMode[] = [];

	for (let index = 0; index < modeCount; index++) {
		modes.push({
			omega: modeOmega[index],
			real: modeReal[index],
			imaginary: modeImag[index],
			linewidth: modeLinewidth[index],
		});
	}

	return { sequence, reading, oscillators, modes };
};


export const decodeFields = (bytes: Uint8Array): FluidFields => {
	const view = header(bytes, FIELD_MAGIC);
	const grid = {
		x: view.getUint32(16, true),
		y: view.getUint32(20, true),
		z: view.getUint32(24, true),
		spacing: view.getFloat32(28, true),
	};
	const cellCount = grid.x * grid.y * grid.z;
	const energyOffset = view.getUint32(48, true);
	const waveRealOffset = view.getUint32(52, true);
	const waveImaginaryOffset = view.getUint32(56, true);
	const byteLength = view.getUint32(60, true);

	if (byteLength !== bytes.byteLength) {
		throw new Error("SFF1 slab byte length does not match its header");
	}

	requireFloatRange(bytes, SLAB_HEADER_BYTES, cellCount * 4, "SFF1 momRho");
	requireFloatRange(bytes, energyOffset, cellCount, "SFF1 energy");
	requireFloatRange(bytes, waveRealOffset, cellCount, "SFF1 wave-real");
	requireFloatRange(
		bytes,
		waveImaginaryOffset,
		cellCount,
		"SFF1 wave-imaginary",
	);

	return {
		sequence: view.getBigUint64(8, true),
		grid,
		momRho: floatView(bytes, SLAB_HEADER_BYTES, cellCount * 4),
		internalEnergy: floatView(bytes, energyOffset, cellCount),
		waveReal: floatView(bytes, waveRealOffset, cellCount),
		waveImaginary: floatView(bytes, waveImaginaryOffset, cellCount),
		densityScale: view.getFloat32(32, true),
		momentumScale: view.getFloat32(36, true),
		energyScale: view.getFloat32(40, true),
		waveScale: view.getFloat32(44, true),
	};
};

export const decodeParticles = (bytes: Uint8Array): FluidParticleFrame => {
	const view = header(bytes, PARTICLE_MAGIC);
	const count = view.getUint32(16, true);
	const stride = view.getUint32(20, true);
	const byteLength = view.getUint32(36, true);

	if (byteLength !== bytes.byteLength) {
		throw new Error("SPF1 slab byte length does not match its header");
	}

	if (stride !== PARTICLE_STRIDE_FLOATS) {
		throw new Error(`SPF1 particle stride must be ${PARTICLE_STRIDE_FLOATS}`);
	}

	requireFloatRange(bytes, SLAB_HEADER_BYTES, count * stride, "SPF1 particles");

	const values = floatView(bytes, SLAB_HEADER_BYTES, count * stride);

	let amplitudeScale = 0;

	for (let index = 11; index < values.length; index += stride) {
		const amplitude = Math.abs(values[index]);
		if (amplitude > amplitudeScale) {
			amplitudeScale = amplitude;
		}
	}

	return new FluidParticleFrame(
		view.getBigUint64(8, true),
		count,
		stride,
		view.getFloat32(24, true),
		view.getFloat32(28, true),
		view.getFloat32(32, true),
		amplitudeScale,
		values,
	);
};
