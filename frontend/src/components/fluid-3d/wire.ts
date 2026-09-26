import type { PhysicsHealthT } from "#/providers/telemetry/telemetry/physics-health";

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

/*
FluidParticleFrame views the manifold's resident particle arrays directly,
exactly as *manifold.State returned them — no interleaved stride, since the
wire carries one Float32Array per field, not a packed struct-of-arrays.
*/
export class FluidParticleFrame {
	constructor(
		readonly sequence: bigint,
		readonly count: number,
		readonly pos: Float32Array,
		readonly vel: Float32Array,
		readonly mass: Float32Array,
		readonly heat: Float32Array,
		readonly energy: Float32Array,
		readonly phase: Float32Array,
		readonly omega: Float32Array,
		readonly amp: Float32Array,
	) {}

	particle(index: number): FluidParticle | null {
		if (index < 0 || index >= this.count) {
			return null;
		}

		const p = index * 3;

		return {
			Position: {
				X: this.pos[p + 0] ?? 0,
				Y: this.pos[p + 1] ?? 0,
				Z: this.pos[p + 2] ?? 0,
			},
			Velocity: {
				X: this.vel[p + 0] ?? 0,
				Y: this.vel[p + 1] ?? 0,
				Z: this.vel[p + 2] ?? 0,
			},
			Mass: this.mass[index] ?? 0,
			Heat: this.heat[index] ?? 0,
			Energy: this.energy[index] ?? 0,
			Phase: this.phase[index] ?? 0,
			Omega: this.omega[index] ?? 0,
			Amplitude: this.amp[index] ?? 0,
		};
	}
}

export type FluidPhaseReading = {
	divergence: number;
	guidanceSpeed: number;
	coherenceMag2: number;
	pressureGradNorm: number;
	viscosityProxy: number;
	kuramotoR: number;
	health?: PhysicsHealthT | null;
	version?: bigint;
	at?: bigint;
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
	amplitude: number;
	side: "bid" | "ask";
};

export type FluidPhase = {
	sequence: bigint;
	reading: FluidPhaseReading;
	oscillators: FluidOscillator[];
	modes: FluidWaveMode[];
};

/*
FluidManifoldFrame is the whole decoded ManifoldFrame: the fields, particles,
and phase views the viewer paints all read from the same one decode, since the
backend now publishes *manifold.State as one value, not three slabs.
*/
export type FluidManifoldFrame = {
	fields: FluidFields;
	particles: FluidParticleFrame;
	phase: FluidPhase;
};
