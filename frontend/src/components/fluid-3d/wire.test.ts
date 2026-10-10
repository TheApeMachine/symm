import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import { Message } from "#/providers/telemetry/telemetry/message";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { IntegratorHealthT } from "#/providers/telemetry/telemetry/integrator-health";
import { ManifoldFrameT } from "#/providers/telemetry/telemetry/manifold-frame";
import { ManifoldReadingT } from "#/providers/telemetry/telemetry/manifold-reading";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { PhaseResultantT } from "#/providers/telemetry/telemetry/phase-resultant";
import { PhysicsHealthT } from "#/providers/telemetry/telemetry/physics-health";
import { WaveModeT } from "#/providers/telemetry/telemetry/wave-mode";
import { decodeManifold, decodeManifoldMeasurement } from "./wire";

const encode = (frame: ManifoldFrameT): Uint8Array => {
	const builder = new flatbuffers.Builder(1024);
	const offset = frame.pack(builder);
	const message = Message.createMessage(
		builder,
		BigInt(0),
		Frame.ManifoldFrame,
		offset,
	);
	Message.finishMessageBuffer(builder, message);
	return builder.asUint8Array();
};

describe("decodeManifold", () => {
	it("decodes one particle's fields, grid, and wave modes from a real ManifoldFrame", () => {
		const health = new PhysicsHealthT(
			new IntegratorHealthT(
				0.01,
				0.01,
				0.01,
				0.01,
				0.01,
				0.001,
				1.234,
				0.02,
				0.02,
				0.02,
				0.02,
				0.02,
				0.01,
				4,
				0,
			),
		);

		const frame = new ManifoldFrameT(
			BigInt(7),
			BigInt(1000),
			BigInt(1),
			BigInt(1),
			[10n],
			[11n],
			[12n],
			[13n],
			[0.1],
			[1],
			[6],
			[4],
			[5],
			[9],
			[0.1, 0.2, 0.3],
			[1, 2, 3],
			[false],
			[false],
			new ManifoldReadingT(1.5, 2.5, 3.5, 4.5, 5.5, 0.75, 0.35, health),
			2,
			2,
			2,
			0.5,
			[9],
			[0.5, 0.75],
			[0.25],
			[-0.25],
			1,
			0.2,
			0.5,
			0.25,
			[new WaveModeT(-2, 0.5, 0.25, 0.3), new WaveModeT(2, -0.5, -0.25, 0.3)],
			[new PhaseResultantT("bid", 1, 9, 0.8, 0.1)],
		);

		frame.pilotVel = [-0.25, 0.5, -0.75];
		health.spatialSigmaRaw = 1.5;
		health.sigmaUniformLimit = true;
		const decoded = decodeManifold(encode(frame));
		expect(Array.from(decoded.particles.pilotVel!)).toEqual([
			-0.25, 0.5, -0.75,
		]);
		expect(decoded.phase.reading.health?.spatialSigmaRaw).toBe(1.5);
		expect(decoded.phase.reading.health?.sigmaUniformLimit).toBe(true);

		expect(decoded.fields.sequence).toBe(7n);
		expect(decoded.fields.grid).toEqual({ x: 2, y: 2, z: 2, spacing: 0.5 });
		expect(Array.from(decoded.fields.momRho)).toEqual([9]);
		expect(Array.from(decoded.fields.internalEnergy)).toEqual([0.5, 0.75]);
		expect(Array.from(decoded.fields.waveReal)).toEqual([0.25]);
		expect(Array.from(decoded.fields.waveImaginary)).toEqual([-0.25]);
		expect(decoded.fields.densityScale).toBe(1);
		expect(decoded.fields.momentumScale).toBeCloseTo(0.2, 5);
		expect(decoded.fields.energyScale).toBe(0.5);
		expect(decoded.fields.waveScale).toBe(0.25);

		expect(decoded.particles.count).toBe(1);
		expect(decoded.particles.particle(0)).toEqual({
			Position: {
				X: expect.closeTo(0.1, 5),
				Y: expect.closeTo(0.2, 5),
				Z: expect.closeTo(0.3, 5),
			},
			Velocity: { X: 1, Y: 2, Z: 3 },
			Mass: 4,
			Heat: 5,
			Energy: 6,
			Phase: expect.closeTo(0.1, 5),
			Omega: 1,
			Amplitude: 9,
		});

		expect(decoded.phase.reading.divergence).toBe(1.5);
		expect(decoded.phase.reading.guidanceSpeed).toBe(2.5);
		expect(decoded.phase.reading.coherenceMag2).toBe(3.5);
		expect(decoded.phase.reading.pressureGradNorm).toBe(4.5);
		expect(decoded.phase.reading.viscosityProxy).toBe(5.5);
		expect(decoded.phase.reading.kuramotoR).toBe(0.75);
		expect(decoded.phase.reading.kuramotoPsi).toBe(0.35);
		expect(decoded.phase.reading.version).toBe(1n);
		expect(decoded.phase.reading.at).toBe(1000n);
		expect(decoded.phase.reading.health?.integrator?.substeps).toBe(4);
		expect(decoded.phase.reading.health?.integrator?.time).toBeCloseTo(
			1.234,
			3,
		);
		expect(decoded.phase.resultants).toHaveLength(1);
		expect(decoded.phase.resultants[0]).toEqual({
			side: "bid",
			count: 1,
			totalAmplitude: 9,
			coherence: 0.8,
			phase: 0.1,
		});
		expect(decoded.phase.oscillators).toHaveLength(1);
		expect(decoded.phase.oscillators[0]).toEqual({
			phase: expect.closeTo(0.1, 5),
			omega: 1,
			amplitude: 9,
			heat: 5,
			side: "bid",
		});
		expect(decoded.phase.modes).toHaveLength(2);
		expect(decoded.phase.modes[0]).toEqual({
			omega: -2,
			real: 0.5,
			imaginary: 0.25,
			linewidth: expect.closeTo(0.3, 5),
		});
		expect(decoded.phase.modes[1]).toEqual({
			omega: 2,
			real: -0.5,
			imaginary: -0.25,
			linewidth: expect.closeTo(0.3, 5),
		});
	});

	it("decodes real physics, health, particles, and modes from an honest MeasurementT", () => {
		const measurement = new MeasurementT();
		measurement.tick = 42n;
		measurement.at = 2000n;
		measurement.metrics = [
			new MetricT("divergence", 0.05),
			new MetricT("guidance_speed", 1.25),
			new MetricT("coherence_mag2", 0.95),
			new MetricT("pressure_grad_norm", 0.12),
			new MetricT("viscosity_proxy", 0.003),
			new MetricT("kuramoto_r", 0.88),
			new MetricT("kuramoto_psi", 1.57),

			// Integrator
			new MetricT("integrator:contact_dt", 0.001),
			new MetricT("integrator:substeps", 8),
			new MetricT("integrator:rejections", 1),

			// Gas
			new MetricT("gas:mass", 100.5),
			new MetricT("gas:max_mach", 0.85),
			new MetricT("gas:vorticity_rms", 0.42),

			// Wave
			new MetricT("wave:norm", 1.05),

			// Pilot
			new MetricT("pilot:density_median", 0.99),

			// Sources
			new MetricT("sources:gas_energy_residual", 1e-6),

			// Scalars
			new MetricT("particle_thermal", 0.5),
			new MetricT("spatial_sigma_raw", 1.2),
			new MetricT("sigma_uniform_limit", 1.0),

			// Particles
			new MetricT("particle_count", 2),
			new MetricT("osc:0:pos_x", 0.1),
			new MetricT("osc:0:pos_y", 0.2),
			new MetricT("osc:0:pos_z", 0.3),
			new MetricT("osc:0:vel_x", 1.0),
			new MetricT("osc:0:vel_y", 2.0),
			new MetricT("osc:0:vel_z", 3.0),
			new MetricT("osc:0:mass", 2.5),
			new MetricT("osc:0:heat", 0.4),
			new MetricT("osc:0:energy", 5.0),
			new MetricT("osc:0:phase", 0.78),
			new MetricT("osc:0:omega", 3.14),
			new MetricT("osc:0:amp", 1.5),
			new MetricT("osc:0:side", 0.0), // bid

			new MetricT("osc:1:pos_x", 0.4),
			new MetricT("osc:1:pos_y", 0.5),
			new MetricT("osc:1:pos_z", 0.6),
			new MetricT("osc:1:vel_x", -1.0),
			new MetricT("osc:1:vel_y", -2.0),
			new MetricT("osc:1:vel_z", -3.0),
			new MetricT("osc:1:mass", 3.0),
			new MetricT("osc:1:heat", 0.8),
			new MetricT("osc:1:energy", 8.0),
			new MetricT("osc:1:phase", 1.57),
			new MetricT("osc:1:omega", 6.28),
			new MetricT("osc:1:amp", 2.0),
			new MetricT("osc:1:side", 1.0), // ask

			// Modes
			new MetricT("mode:0:omega", 10.0),
			new MetricT("mode:0:real", 0.7),
			new MetricT("mode:0:imag", 0.1),
			new MetricT("mode:0:linewidth", 0.05),

			// Resultants
			new MetricT("resultant:bid:count", 1),
			new MetricT("resultant:bid:amplitude", 1.5),
			new MetricT("resultant:bid:coherence", 0.9),
			new MetricT("resultant:bid:phase", 0.78),

			new MetricT("resultant:ask:count", 1),
			new MetricT("resultant:ask:amplitude", 2.0),
			new MetricT("resultant:ask:coherence", 0.85),
			new MetricT("resultant:ask:phase", 1.57),

			// Grid
			new MetricT("grid_x", 16),
			new MetricT("grid_y", 16),
			new MetricT("grid_z", 16),
			new MetricT("grid_spacing", 0.0625),
			new MetricT("density_scale", 1.0),
		];

		const decoded = decodeManifoldMeasurement(measurement);

		expect(decoded.fields.sequence).toBe(42n);
		expect(decoded.fields.grid).toEqual({ x: 16, y: 16, z: 16, spacing: 0.0625 });
		expect(decoded.particles.count).toBe(2);

		const p0 = decoded.particles.particle(0);
		expect(p0?.Position.X).toBeCloseTo(0.1, 5);
		expect(p0?.Position.Y).toBeCloseTo(0.2, 5);
		expect(p0?.Position.Z).toBeCloseTo(0.3, 5);
		expect(p0?.Velocity).toEqual({ X: 1.0, Y: 2.0, Z: 3.0 });
		expect(p0?.Mass).toBe(2.5);
		expect(p0?.Heat).toBeCloseTo(0.4, 5);
		expect(p0?.Energy).toBe(5.0);
		expect(p0?.Phase).toBeCloseTo(0.78, 5);
		expect(p0?.Omega).toBeCloseTo(3.14, 5);
		expect(p0?.Amplitude).toBe(1.5);

		const p1 = decoded.particles.particle(1);
		expect(p1?.Position.X).toBeCloseTo(0.4, 5);
		expect(p1?.Position.Y).toBeCloseTo(0.5, 5);
		expect(p1?.Position.Z).toBeCloseTo(0.6, 5);
		expect(p1?.Velocity).toEqual({ X: -1.0, Y: -2.0, Z: -3.0 });

		// Phase reading & health
		expect(decoded.phase.reading.divergence).toBe(0.05);
		expect(decoded.phase.reading.guidanceSpeed).toBe(1.25);
		expect(decoded.phase.reading.coherenceMag2).toBe(0.95);
		expect(decoded.phase.reading.kuramotoR).toBe(0.88);
		expect(decoded.phase.reading.health?.integrator?.substeps).toBe(8);
		expect(decoded.phase.reading.health?.integrator?.rejections).toBe(1);
		expect(decoded.phase.reading.health?.gas?.mass).toBe(100.5);
		expect(decoded.phase.reading.health?.gas?.maxMach).toBe(0.85);
		expect(decoded.phase.reading.health?.wave?.norm).toBe(1.05);
		expect(decoded.phase.reading.health?.pilot?.densityMedian).toBe(0.99);
		expect(decoded.phase.reading.health?.sources?.gasEnergyResidual).toBe(1e-6);
		expect(decoded.phase.reading.health?.sigmaUniformLimit).toBe(true);

		// Oscillators & resultants
		expect(decoded.phase.oscillators).toHaveLength(2);
		expect(decoded.phase.oscillators[0]?.side).toBe("bid");
		expect(decoded.phase.oscillators[1]?.side).toBe("ask");

		expect(decoded.phase.resultants).toHaveLength(2);
		expect(decoded.phase.resultants.find((r) => r.side === "bid")?.totalAmplitude).toBe(1.5);
		expect(decoded.phase.resultants.find((r) => r.side === "ask")?.coherence).toBe(0.85);

		// Modes
		expect(decoded.phase.modes).toHaveLength(1);
		expect(decoded.phase.modes[0]?.omega).toBe(10.0);
	});
});
