import * as flatbuffers from "flatbuffers";
import { GasHealthT } from "#/providers/telemetry/telemetry/gas-health";
import { IntegratorHealthT } from "#/providers/telemetry/telemetry/integrator-health";
import { ManifoldFrame } from "#/providers/telemetry/telemetry/manifold-frame";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { Message } from "#/providers/telemetry/telemetry/message";
import { PhaseResultant as PhaseResultantTable } from "#/providers/telemetry/telemetry/phase-resultant";
import {
	PhysicsHealth,
	PhysicsHealthT,
} from "#/providers/telemetry/telemetry/physics-health";
import { PilotHealthT } from "#/providers/telemetry/telemetry/pilot-health";
import { SourceLedgerT } from "#/providers/telemetry/telemetry/source-ledger";
import { WaveHealthT } from "#/providers/telemetry/telemetry/wave-health";
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
		readonly pilotVel: Float32Array | null,
	) {}

	particle(index: number): FluidParticle | null {
		if (index < 0 || index >= this.count) {
			return null;
		}

		const p = index * 3;

		const posX = this.pos[p + 0];
		const posY = this.pos[p + 1];
		const posZ = this.pos[p + 2];
		const velX = this.vel[p + 0];
		const velY = this.vel[p + 1];
		const velZ = this.vel[p + 2];
		const mass = this.mass[index];
		const heat = this.heat[index];
		const energy = this.energy[index];
		const phase = this.phase[index];
		const omega = this.omega[index];
		const amp = this.amp[index];

		if (
			posX === undefined ||
			posY === undefined ||
			posZ === undefined ||
			velX === undefined ||
			velY === undefined ||
			velZ === undefined ||
			mass === undefined ||
			heat === undefined ||
			energy === undefined ||
			phase === undefined ||
			omega === undefined ||
			amp === undefined
		) {
			return null;
		}

		return {
			Position: { X: posX, Y: posY, Z: posZ },
			Velocity: { X: velX, Y: velY, Z: velZ },
			Mass: mass,
			Heat: heat,
			Energy: energy,
			Phase: phase,
			Omega: omega,
			Amplitude: amp,
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
	kuramotoPsi: number;
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
	heat: number;
	side: "bid" | "ask";
};

export type FluidResultant = {
	side: "bid" | "ask";
	count: number;
	totalAmplitude: number;
	coherence: number;
	phase: number;
};

export type FluidPhase = {
	sequence: bigint;
	reading: FluidPhaseReading;
	oscillators: FluidOscillator[];
	modes: FluidWaveMode[];
	resultants: FluidResultant[];
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
const healthTable = new PhysicsHealth();
const resultantObj = new PhaseResultantTable();

/*
decodeManifold reads one ManifoldFrame flatbuffer as published on the hub
WebSocket (types.EncodeManifold → ui.UITee → /ws). bytes is one complete
SYMM-identified Message carrying FrameManifoldFrame.
*/
export const decodeManifold = (bytes: Uint8Array): FluidManifoldFrame => {
	const buffer = new flatbuffers.ByteBuffer(bytes);

	if (!Message.bufferHasIdentifier(buffer)) {
		throw new Error("manifold frame is missing its message identifier");
	}

	const message = Message.getRootAsMessage(buffer);
	const frame = message.frame(new ManifoldFrame());

	if (frame === null) {
		throw new Error("message does not carry a ManifoldFrame");
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

	const posArray = frame.posArray();
	const velArray = frame.velArray();
	const massArray = frame.massArray();
	const heatArray = frame.heatArray();
	const energyArray = frame.energyArray();
	const phaseArray = frame.phaseArray();
	const omegaArray = frame.omegaArray();
	const amplitudeArray = frame.ampArray();

	if (
		count > 0 &&
		(!posArray ||
			!velArray ||
			!massArray ||
			!heatArray ||
			!energyArray ||
			!phaseArray ||
			!omegaArray ||
			!amplitudeArray)
	) {
		throw new Error(
			"manifold frame missing required particle arrays for non-zero count",
		);
	}

	const particles = new FluidParticleFrame(
		sequence,
		count,
		posArray ?? new Float32Array(0),
		velArray ?? new Float32Array(0),
		massArray ?? new Float32Array(0),
		heatArray ?? new Float32Array(0),
		energyArray ?? new Float32Array(0),
		phaseArray ?? new Float32Array(0),
		omegaArray ?? new Float32Array(0),
		amplitudeArray ?? new Float32Array(0),
		frame.pilotVelArray(),
	);

	const oscillators: FluidOscillator[] = [];

	for (let index = 0; index < count; index += 1) {
		const tokenID = frame.tokenIds(index);
		if (tokenID === null) {
			throw new Error(`manifold frame missing token ID at index ${index}`);
		}
		if (!phaseArray || !omegaArray || !amplitudeArray || !heatArray) {
			throw new Error(
				`manifold frame missing oscillator arrays at index ${index}`,
			);
		}

		const p = phaseArray[index];
		const o = omegaArray[index];
		const a = amplitudeArray[index];
		const h = heatArray[index];

		if (
			p === undefined ||
			o === undefined ||
			a === undefined ||
			h === undefined
		) {
			throw new Error(`missing oscillator values at index ${index}`);
		}

		oscillators.push({
			phase: p,
			omega: o,
			amplitude: a,
			heat: h,
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

	const resultants: FluidResultant[] = [];

	for (let index = 0; index < frame.resultantsLength(); index += 1) {
		const res = frame.resultants(index, resultantObj);

		if (res === null) {
			continue;
		}

		resultants.push({
			side: res.side() === "ask" ? "ask" : "bid",
			count: res.count(),
			totalAmplitude: res.totalAmplitude(),
			coherence: res.coherence(),
			phase: res.phase(),
		});
	}

	const readingHealth = reading.health(healthTable);
	const health = readingHealth !== null ? readingHealth.unpack() : null;

	const phase: FluidPhase = {
		sequence,
		reading: {
			divergence: reading.divergence(),
			guidanceSpeed: reading.guidanceSpeed(),
			coherenceMag2: reading.coherenceMag2(),
			pressureGradNorm: reading.pressureGradNorm(),
			viscosityProxy: reading.viscosityProxy(),
			kuramotoR: reading.kuramotoR(),
			kuramotoPsi: reading.kuramotoPsi(),
			health,
			version: frame.version(),
			at: frame.at(),
		},
		oscillators,
		modes,
		resultants,
	};

	return { fields, particles, phase };
};

export const decodeManifoldMeasurement = (
	measurement: MeasurementT,
): FluidManifoldFrame => {
	if (!measurement) {
		throw new Error("manifold measurement is null or undefined");
	}

	const sequence = measurement.tick;
	const at = measurement.at;
	const version = measurement.tick;

	const integrator = new IntegratorHealthT();
	const gas = new GasHealthT();
	const wave = new WaveHealthT();
	const pilot = new PilotHealthT();
	const sources = new SourceLedgerT();
	const health = new PhysicsHealthT(integrator, gas, wave, pilot, sources);

	let divergence = 0;
	let guidanceSpeed = 0;
	let coherenceMag2 = 0;
	let pressureGradNorm = 0;
	let viscosityProxy = 0;
	let kuramotoR = 0;
	let kuramotoPsi = 0;

	let gridX = 0;
	let gridY = 0;
	let gridZ = 0;
	let gridSpacing = 0;
	let densityScale = 1;
	let momentumScale = 1;
	let energyScale = 1;
	let waveScale = 1;

	let particleCount = 0;
	const metrics = measurement.metrics;
	const metricsLen = metrics.length;

	for (let i = 0; i < metricsLen; i += 1) {
		const metric = metrics[i];
		if (metric && metric.name === "particle_count") {
			particleCount = Math.round(metric.raw);
			break;
		}
	}

	const count = particleCount;
	const pos = new Float32Array(count * 3);
	const vel = new Float32Array(count * 3);
	const mass = new Float32Array(count);
	const heat = new Float32Array(count);
	const energy = new Float32Array(count);
	const phase = new Float32Array(count);
	const omega = new Float32Array(count);
	const amp = new Float32Array(count);
	const pilotVel = new Float32Array(count * 3);
	const sides: ("bid" | "ask")[] = new Array(count);
	let hasPilot = false;

	const modes: FluidWaveMode[] = [];
	let bidCount = 0;
	let bidAmp = 0;
	let bidCoherence = 0;
	let bidPhase = 0;
	let hasBid = false;

	let askCount = 0;
	let askAmp = 0;
	let askCoherence = 0;
	let askPhase = 0;
	let hasAsk = false;

	for (let i = 0; i < metricsLen; i += 1) {
		const metric = metrics[i];
		if (!metric || metric.name === null) continue;

		const name = typeof metric.name === "string" ? metric.name : "";
		const val = metric.raw;

		switch (name) {
			case "divergence":
				divergence = val;
				break;
			case "guidance_speed":
				guidanceSpeed = val;
				break;
			case "coherence_mag2":
				coherenceMag2 = val;
				break;
			case "pressure_grad_norm":
				pressureGradNorm = val;
				break;
			case "viscosity_proxy":
				viscosityProxy = val;
				break;
			case "kuramoto_r":
				kuramotoR = val;
				break;
			case "kuramoto_psi":
				kuramotoPsi = val;
				break;

			// Integrator
			case "integrator:contact_dt":
				integrator.contactDt = val;
				break;
			case "integrator:requested_dt":
				integrator.requestedDt = val;
				break;
			case "integrator:target_dt":
				integrator.targetDt = val;
				break;
			case "integrator:accepted_dt":
				integrator.acceptedDt = val;
				break;
			case "integrator:last_dt":
				integrator.lastDt = val;
				break;
			case "integrator:min_dt":
				integrator.minDt = val;
				break;
			case "integrator:time":
				integrator.time = val;
				break;
			case "integrator:hyperbolic_dt":
				integrator.hyperbolicDt = val;
				break;
			case "integrator:viscous_dt":
				integrator.viscousDt = val;
				break;
			case "integrator:thermal_dt":
				integrator.thermalDt = val;
				break;
			case "integrator:particle_dt":
				integrator.particleDt = val;
				break;
			case "integrator:phase_dt":
				integrator.phaseDt = val;
				break;
			case "integrator:combined_dt":
				integrator.combinedDt = val;
				break;
			case "integrator:substeps":
				integrator.substeps = Math.round(val);
				break;
			case "integrator:rejections":
				integrator.rejections = Math.round(val);
				break;

			// Gas
			case "gas:mass":
				gas.mass = val;
				break;
			case "gas:internal":
				gas.internal = val;
				break;
			case "gas:kinetic":
				gas.kinetic = val;
				break;
			case "gas:total":
				gas.total = val;
				break;
			case "gas:min_density":
				gas.minDensity = val;
				break;
			case "gas:min_pressure":
				gas.minPressure = val;
				break;
			case "gas:min_temperature":
				gas.minTemperature = val;
				break;
			case "gas:max_speed":
				gas.maxSpeed = val;
				break;
			case "gas:max_sound":
				gas.maxSound = val;
				break;
			case "gas:max_mach":
				gas.maxMach = val;
				break;
			case "gas:vorticity_rms":
				gas.vorticityRms = val;
				break;
			case "gas:vorticity_max":
				gas.vorticityMax = val;
				break;
			case "gas:strain_rms":
				gas.strainRms = val;
				break;
			case "gas:strain_max":
				gas.strainMax = val;
				break;
			case "gas:viscous_power":
				gas.viscousPower = val;
				break;

			// Wave
			case "wave:norm":
				wave.norm = val;
				break;
			case "wave:kinetic":
				wave.kinetic = val;
				break;
			case "wave:potential":
				wave.potential = val;
				break;
			case "wave:nonlinear":
				wave.nonlinear = val;
				break;
			case "wave:chemical":
				wave.chemical = val;
				break;
			case "wave:projected_norm":
				wave.projectedNorm = val;
				break;
			case "wave:phase_potential":
				wave.phasePotential = val;
				break;

			// Pilot
			case "pilot:density_p01":
				pilot.densityP01 = val;
				break;
			case "pilot:density_p10":
				pilot.densityP10 = val;
				break;
			case "pilot:density_median":
				pilot.densityMedian = val;
				break;
			case "pilot:integration_error_max":
				pilot.integrationErrorMax = val;
				break;
			case "pilot:speed_rms":
				pilot.speedRms = val;
				break;
			case "pilot:speed_max":
				pilot.speedMax = val;
				break;
			case "pilot:displacement_rms":
				pilot.displacementRms = val;
				break;
			case "pilot:displacement_max":
				pilot.displacementMax = val;
				break;
			case "pilot:min_density":
				pilot.minDensity = val;
				break;

			// Sources
			case "sources:gas_energy_residual":
				sources.gasEnergyResidual = val;
				break;
			case "sources:conservative_wave_error":
				sources.conservativeWaveError = val;
				break;
			case "sources:pic_deposit_energy_residual":
				sources.picDepositEnergyResidual = val;
				break;
			case "sources:particle_balance_residual":
				sources.particleBalanceResidual = val;
				break;
			case "sources:gravity_balance_residual":
				sources.gravityBalanceResidual = val;
				break;

			// Scalar Health Fields
			case "particle_thermal":
				health.particleThermal = val;
				break;
			case "particle_oscillator":
				health.particleOscillator = val;
				break;
			case "particle_kinetic":
				health.particleKinetic = val;
				break;
			case "particle_material_total":
				health.particleMaterialTotal = val;
				break;
			case "spatial_sigma_raw":
				health.spatialSigmaRaw = val;
				break;
			case "spatial_sigma_used":
				health.spatialSigmaUsed = val;
				break;
			case "sigma_uniform_limit":
				health.sigmaUniformLimit = val > 0.5;
				break;

			// Grid
			case "grid_x":
				gridX = Math.round(val);
				break;
			case "grid_y":
				gridY = Math.round(val);
				break;
			case "grid_z":
				gridZ = Math.round(val);
				break;
			case "grid_spacing":
				gridSpacing = val;
				break;
			case "density_scale":
				densityScale = val;
				break;
			case "momentum_scale":
				momentumScale = val;
				break;
			case "energy_scale":
				energyScale = val;
				break;
			case "wave_scale":
				waveScale = val;
				break;

			case "particle_count":
				break;

			default:
				if (name.startsWith("osc:")) {
					const c1 = 4;
					const c2 = name.indexOf(":", c1);
					if (c2 === -1) {
						throw new Error(`malformed osc metric name: ${name}`);
					}
					const idx = parseInt(name.substring(c1, c2), 10);
					if (Number.isNaN(idx) || idx < 0 || idx >= count) {
						throw new Error(`osc index ${idx} out of range [0, ${count})`);
					}
					const prop = name.substring(c2 + 1);
					const p = idx * 3;
					switch (prop) {
						case "pos_x":
							pos[p + 0] = val;
							break;
						case "pos_y":
							pos[p + 1] = val;
							break;
						case "pos_z":
							pos[p + 2] = val;
							break;
						case "vel_x":
							vel[p + 0] = val;
							break;
						case "vel_y":
							vel[p + 1] = val;
							break;
						case "vel_z":
							vel[p + 2] = val;
							break;
						case "mass":
							mass[idx] = val;
							break;
						case "heat":
							heat[idx] = val;
							break;
						case "energy":
							energy[idx] = val;
							break;
						case "phase":
							phase[idx] = val;
							break;
						case "omega":
							omega[idx] = val;
							break;
						case "amp":
							amp[idx] = val;
							break;
						case "side":
							sides[idx] = val > 0.5 ? "ask" : "bid";
							break;
						case "pilot_vx":
							pilotVel[p + 0] = val;
							hasPilot = true;
							break;
						case "pilot_vy":
							pilotVel[p + 1] = val;
							hasPilot = true;
							break;
						case "pilot_vz":
							pilotVel[p + 2] = val;
							hasPilot = true;
							break;
						default:
							throw new Error(`unknown osc property: ${prop}`);
					}
				} else if (name.startsWith("mode:")) {
					const c1 = 5;
					const c2 = name.indexOf(":", c1);
					if (c2 === -1) {
						throw new Error(`malformed mode metric name: ${name}`);
					}
					const idx = parseInt(name.substring(c1, c2), 10);
					if (Number.isNaN(idx) || idx < 0) {
						throw new Error(`invalid mode index: ${idx}`);
					}
					while (modes.length <= idx) {
						modes.push({ omega: 0, real: 0, imaginary: 0, linewidth: 0 });
					}
					const m = modes[idx];
					if (!m) {
						throw new Error(`mode at index ${idx} not initialized`);
					}
					const prop = name.substring(c2 + 1);
					switch (prop) {
						case "omega":
							m.omega = val;
							break;
						case "real":
							m.real = val;
							break;
						case "imag":
							m.imaginary = val;
							break;
						case "linewidth":
							m.linewidth = val;
							break;
						default:
							throw new Error(`unknown mode property: ${prop}`);
					}
				} else if (name.startsWith("resultant:bid:")) {
					hasBid = true;
					const prop = name.substring(14);
					switch (prop) {
						case "count":
							bidCount = Math.round(val);
							break;
						case "amplitude":
							bidAmp = val;
							break;
						case "coherence":
							bidCoherence = val;
							break;
						case "phase":
							bidPhase = val;
							break;
						default:
							throw new Error(`unknown resultant property: ${prop}`);
					}
				} else if (name.startsWith("resultant:ask:")) {
					hasAsk = true;
					const prop = name.substring(14);
					switch (prop) {
						case "count":
							askCount = Math.round(val);
							break;
						case "amplitude":
							askAmp = val;
							break;
						case "coherence":
							askCoherence = val;
							break;
						case "phase":
							askPhase = val;
							break;
						default:
							throw new Error(`unknown resultant property: ${prop}`);
					}
				}
				break;
		}
	}

	const oscillators: FluidOscillator[] = new Array(count);
	for (let i = 0; i < count; i += 1) {
		const side = sides[i];
		if (!side) {
			throw new Error(`missing side classification for particle ${i}`);
		}
		const p = phase[i];
		const o = omega[i];
		const a = amp[i];
		const h = heat[i];
		if (
			p === undefined ||
			o === undefined ||
			a === undefined ||
			h === undefined
		) {
			throw new Error(`missing particle values at index ${i}`);
		}
		oscillators[i] = {
			phase: p,
			omega: o,
			amplitude: a,
			heat: h,
			side,
		};
	}

	const resultants: FluidResultant[] = [];
	if (hasBid) {
		resultants.push({
			side: "bid",
			count: bidCount,
			totalAmplitude: bidAmp,
			coherence: bidCoherence,
			phase: bidPhase,
		});
	}
	if (hasAsk) {
		resultants.push({
			side: "ask",
			count: askCount,
			totalAmplitude: askAmp,
			coherence: askCoherence,
			phase: askPhase,
		});
	}

	const particles = new FluidParticleFrame(
		sequence,
		count,
		pos,
		vel,
		mass,
		heat,
		energy,
		phase,
		omega,
		amp,
		hasPilot ? pilotVel : null,
	);

	const fluidPhase: FluidPhase = {
		sequence,
		reading: {
			divergence,
			guidanceSpeed,
			coherenceMag2,
			pressureGradNorm,
			viscosityProxy,
			kuramotoR,
			kuramotoPsi,
			health,
			version,
			at,
		},
		oscillators,
		modes,
		resultants,
	};

	const fields: FluidFields = {
		sequence,
		grid: {
			x: gridX,
			y: gridY,
			z: gridZ,
			spacing: gridSpacing,
		},
		momRho: new Float32Array(0),
		internalEnergy: new Float32Array(0),
		waveReal: new Float32Array(0),
		waveImaginary: new Float32Array(0),
		densityScale,
		momentumScale,
		energyScale,
		waveScale,
	};

	return { fields, particles, phase: fluidPhase };
};
