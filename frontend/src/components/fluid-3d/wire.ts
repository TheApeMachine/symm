import * as flatbuffers from "flatbuffers";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";
import { ManifoldFrame } from "#/providers/telemetry/telemetry/manifold-frame";
import { WaveMode as WaveModeTable } from "#/providers/telemetry/telemetry/wave-mode";

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

const modeObj = new WaveModeTable();

/*
decodeManifold reads one ManifoldFrame flatbuffer, exactly as
logic/manifold.Solver.Step returned it and ui/webrtc.go's encodeManifold
mirrored it, field for field. bytes is one complete WebRTC record payload
(the FluidRecordReader already stripped the SFD1 record framing).
*/
export const decodeManifold = (bytes: Uint8Array): FluidManifoldFrame => {
	const buffer = new flatbuffers.ByteBuffer(bytes);

	if (!Envelope.bufferHasIdentifier(buffer)) {
		throw new Error("manifold frame is missing its envelope identifier");
	}

	const envelope = Envelope.getRootAsEnvelope(buffer);
	const frame = envelope.frame(new ManifoldFrame());

	if (frame === null) {
		throw new Error("envelope does not carry a ManifoldFrame");
	}

	const sequence = frame.sequence();
	const count = Number(frame.n());

	const reading = frame.reading();

	if (reading === null) {
		throw new Error("ManifoldFrame is missing its reading");
	}

	const fields: FluidFields = {
		sequence,
		grid: {
			x: frame.gridX(),
			y: frame.gridY(),
			z: frame.gridZ(),
			spacing: frame.gridSpacing(),
		},
		momRho: frame.momRhoArray() ?? new Float32Array(0),
		internalEnergy: frame.fieldEnergyArray() ?? new Float32Array(0),
		waveReal: frame.waveRealArray() ?? new Float32Array(0),
		waveImaginary: frame.waveImagArray() ?? new Float32Array(0),
		densityScale: frame.densityScale(),
		momentumScale: frame.momentumScale(),
		energyScale: frame.energyScale(),
		waveScale: frame.waveScale(),
	};

	const particles = new FluidParticleFrame(
		sequence,
		count,
		frame.posArray() ?? new Float32Array(0),
		frame.velArray() ?? new Float32Array(0),
		frame.massArray() ?? new Float32Array(0),
		frame.heatArray() ?? new Float32Array(0),
		frame.energyArray() ?? new Float32Array(0),
		frame.phaseArray() ?? new Float32Array(0),
		frame.omegaArray() ?? new Float32Array(0),
		frame.ampArray() ?? new Float32Array(0),
	);

	const oscillators: FluidOscillator[] = [];
	const phaseArray = frame.phaseArray();
	const omegaArray = frame.omegaArray();
	const amplitudeArray = frame.ampArray();

	for (let index = 0; index < count; index += 1) {
		const tokenID = frame.tokenIds(index) ?? 0n;

		oscillators.push({
			phase: phaseArray?.[index] ?? 0,
			omega: omegaArray?.[index] ?? 0,
			amplitude: amplitudeArray?.[index] ?? 0,
			side: (tokenID & 1n) === 0n ? "bid" : "ask",
		});
	}

	const modes: FluidWaveMode[] = [];

	for (let index = 0; index < frame.modesLength(); index += 1) {
		const mode = frame.modes(index, modeObj);

		if (mode === null) {
			continue;
		}

		modes.push({
			omega: mode.omega(),
			real: mode.real(),
			imaginary: mode.imaginary(),
			linewidth: mode.linewidth(),
		});
	}

	const phase: FluidPhase = {
		sequence,
		reading: {
			divergence: reading.divergence(),
			guidanceSpeed: reading.guidanceSpeed(),
			coherenceMag2: reading.coherenceMag2(),
			pressureGradNorm: reading.pressureGradNorm(),
			viscosityProxy: reading.viscosityProxy(),
			kuramotoR: reading.kuramotoR(),
		},
		oscillators,
		modes,
	};

	return { fields, particles, phase };
};
