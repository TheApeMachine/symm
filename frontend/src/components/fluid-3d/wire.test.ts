import { describe, expect, it } from "vitest";
import { Value } from "#/types/value";
import { decodeManifoldMeasurement } from "./wire";

describe("decodeManifoldMeasurement", () => {
	it("decodes real physics, health, particles, and modes from an honest Value", () => {
		const val = new Value({
			tick: 42n,
			timestamp: 2000n,
			metrics: [
				{ name: "divergence", raw: 0.05 },
				{ name: "guidance_speed", raw: 1.25 },
				{ name: "coherence_mag2", raw: 0.95 },
				{ name: "pressure_grad_norm", raw: 0.12 },
				{ name: "viscosity_proxy", raw: 0.003 },
				{ name: "kuramoto_r", raw: 0.88 },
				{ name: "kuramoto_psi", raw: 1.57 },

				// Integrator
				{ name: "integrator:contact_dt", raw: 0.001 },
				{ name: "integrator:substeps", raw: 8 },
				{ name: "integrator:rejections", raw: 1 },

				// Gas
				{ name: "gas:mass", raw: 100.5 },
				{ name: "gas:max_mach", raw: 0.85 },
				{ name: "gas:vorticity_rms", raw: 0.42 },

				// Wave
				{ name: "wave:norm", raw: 1.05 },

				// Pilot
				{ name: "pilot:density_median", raw: 0.99 },

				// Sources
				{ name: "sources:gas_energy_residual", raw: 1e-6 },

				// Scalars
				{ name: "particle_thermal", raw: 0.5 },
				{ name: "spatial_sigma_raw", raw: 1.2 },
				{ name: "sigma_uniform_limit", raw: 1.0 },

				// Particles
				{ name: "particle_count", raw: 2 },
				{ name: "osc:0:pos_x", raw: 0.1 },
				{ name: "osc:0:pos_y", raw: 0.2 },
				{ name: "osc:0:pos_z", raw: 0.3 },
				{ name: "osc:0:vel_x", raw: 1.0 },
				{ name: "osc:0:vel_y", raw: 2.0 },
				{ name: "osc:0:vel_z", raw: 3.0 },
				{ name: "osc:0:mass", raw: 2.5 },
				{ name: "osc:0:heat", raw: 0.4 },
				{ name: "osc:0:energy", raw: 5.0 },
				{ name: "osc:0:phase", raw: 0.78 },
				{ name: "osc:0:omega", raw: 3.14 },
				{ name: "osc:0:amp", raw: 1.5 },
				{ name: "osc:0:side", raw: 0.0 }, // bid

				{ name: "osc:1:pos_x", raw: 0.4 },
				{ name: "osc:1:pos_y", raw: 0.5 },
				{ name: "osc:1:pos_z", raw: 0.6 },
				{ name: "osc:1:vel_x", raw: -1.0 },
				{ name: "osc:1:vel_y", raw: -2.0 },
				{ name: "osc:1:vel_z", raw: -3.0 },
				{ name: "osc:1:mass", raw: 3.0 },
				{ name: "osc:1:heat", raw: 0.8 },
				{ name: "osc:1:energy", raw: 8.0 },
				{ name: "osc:1:phase", raw: 1.57 },
				{ name: "osc:1:omega", raw: 6.28 },
				{ name: "osc:1:amp", raw: 2.0 },
				{ name: "osc:1:side", raw: 1.0 }, // ask

				// Modes
				{ name: "mode:0:omega", raw: 10.0 },
				{ name: "mode:0:real", raw: 0.7 },
				{ name: "mode:0:imag", raw: 0.1 },
				{ name: "mode:0:linewidth", raw: 0.05 },

				// Resultants
				{ name: "resultant:bid:count", raw: 1 },
				{ name: "resultant:bid:amplitude", raw: 1.5 },
				{ name: "resultant:bid:coherence", raw: 0.9 },
				{ name: "resultant:bid:phase", raw: 0.78 },

				{ name: "resultant:ask:count", raw: 1 },
				{ name: "resultant:ask:amplitude", raw: 2.0 },
				{ name: "resultant:ask:coherence", raw: 0.85 },
				{ name: "resultant:ask:phase", raw: 1.57 },

				// Grid
				{ name: "grid_x", raw: 16 },
				{ name: "grid_y", raw: 16 },
				{ name: "grid_z", raw: 16 },
				{ name: "grid_spacing", raw: 0.0625 },
				{ name: "density_scale", raw: 1.0 },
			],
		});

		const decoded = decodeManifoldMeasurement(val);

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
