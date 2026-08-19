import * as THREE from "three";
import type { FluidParticle } from "./wire";

const particleVertexShader = /* glsl */ `
	attribute float aHeat;
	attribute float aEnergy;
	attribute float aMass;
	attribute float aPhase;
	uniform float uProjectionScale;
	uniform float uPointDiameter;
	varying float vHeat;
	varying float vEnergy;
	varying float vPhase;

	void main() {
		vec4 viewPosition = modelViewMatrix * vec4(position, 1.0);
		vHeat = aHeat;
		vEnergy = aEnergy;
		vPhase = aPhase;
		// Modulate point size cleanly within unit box scale
		float energyScale = 0.8 + 0.5 * clamp(aEnergy, 0.0, 1.0);
		float massScale = 0.8 + 0.6 * clamp(aMass, 0.0, 1.0);
		gl_PointSize = clamp(
			energyScale * massScale * uPointDiameter * uProjectionScale /
				max(-viewPosition.z, 0.0001),
			3.0,
			48.0
		);
		gl_Position = projectionMatrix * viewPosition;
	}
`;

const particleFragmentShader = /* glsl */ `
	varying float vHeat;
	varying float vEnergy;
	varying float vPhase;

	const float PI = 3.141592653589793;

	vec3 phaseColor(float phase) {
		vec3 phaseOffsets = vec3(0.0, 2.0 * PI / 3.0, 4.0 * PI / 3.0);
		return 0.5 + 0.5 * cos(phase + phaseOffsets);
	}

	void main() {
		float radius = length(gl_PointCoord - vec2(0.5));

		if (radius > 0.5) {
			discard;
		}

		float glow = smoothstep(0.5, 0.0, radius);
		float core = smoothstep(0.3, 0.0, radius);

		// Geometric domain: thermodynamic heat temperature. In the kernel the
		// particle's heat is the entropic store Q (metabolic bank): it pays the
		// coupling work W = metabolic_rate·A²·dt and is re-filled from the gas
		// field temperature T = e_int/(ρ·c_v). The inner circle shows this
		// particle identity alone — no wave contribution.
		float heat = clamp(vHeat, 0.0, 1.0);
		vec3 cold = vec3(0.1, 0.45, 0.9);
		vec3 warm = vec3(1.0, 0.45, 0.08);
		vec3 hot = vec3(1.0, 0.95, 0.8);
		vec3 thermoColor = heat < 0.5
			? mix(cold, warm, heat * 2.0)
			: mix(warm, hot, (heat - 0.5) * 2.0);

		// Wave domain: oscillator phase angle hue, confined to the outer ring
		// so the coherence identity never contaminates the thermal core.
		vec3 waveColor = phaseColor(vPhase);

		// Inner circle = geometric heat only; outer ring = wave phase hue.
		vec3 color = mix(waveColor, thermoColor, core);

		// Outer resonance ring reflecting wave energy (A²)
		float energyRing = smoothstep(0.48, 0.38, radius) *
			smoothstep(0.28, 0.38, radius) * clamp(vEnergy, 0.0, 1.0);

		float brightness = mix(0.75, 1.4, heat) * glow;
		gl_FragColor = vec4(color * brightness + waveColor * energyRing * 1.5, glow * 0.9 + core * 0.1);
	}
`;

const finite = (value: number, name: string, index: number) => {
	if (!Number.isFinite(value)) {
		throw new Error(`particle ${index} ${name} is not finite`);
	}

	return value;
};

/*
FluidParticles is one GPU point buffer and one draw call for the complete
resident particle selection. No particle owns a mesh or expanded geometry.
*/
export class FluidParticles {
	readonly points: THREE.Points;
	private particles: FluidParticle[] = [];
	private readonly material: THREE.ShaderMaterial;

	constructor() {
		this.material = new THREE.ShaderMaterial({
			uniforms: {
				uProjectionScale: { value: 1 },
				uPointDiameter: { value: 1 / 64 },
			},
			vertexShader: particleVertexShader,
			fragmentShader: particleFragmentShader,
			transparent: true,
			depthWrite: false,
			blending: THREE.AdditiveBlending,
		});
		this.points = new THREE.Points(new THREE.BufferGeometry(), this.material);
		this.points.renderOrder = 2;
	}

	update(particles: FluidParticle[]) {
		const positions = new Float32Array(particles.length * 3);
		const heat = new Float32Array(particles.length);
		const energy = new Float32Array(particles.length);
		const mass = new Float32Array(particles.length);
		const phase = new Float32Array(particles.length);
		let maximumHeat = 0;
		let maximumEnergy = 0;
		let maximumMass = 0;

		for (let index = 0; index < particles.length; index += 1) {
			const particle = particles[index];

			if (particle === undefined) {
				throw new Error(`particle ${index} is missing`);
			}

			const offset = index * 3;
			positions[offset] = finite(particle.Position.X, "Position.X", index);
			positions[offset + 1] = finite(particle.Position.Y, "Position.Y", index);
			positions[offset + 2] = finite(particle.Position.Z, "Position.Z", index);
			heat[index] = finite(particle.Heat, "Heat", index);
			energy[index] = finite(particle.Energy, "Energy", index);
			mass[index] = finite(particle.Mass, "Mass", index);
			phase[index] = finite(particle.Phase, "Phase", index);
			maximumHeat = Math.max(maximumHeat, Math.abs(heat[index]));
			maximumEnergy = Math.max(maximumEnergy, Math.abs(energy[index]));
			maximumMass = Math.max(maximumMass, Math.abs(mass[index]));
		}

		if (maximumHeat > 0) {
			for (let index = 0; index < heat.length; index += 1) {
				heat[index] /= maximumHeat;
			}
		}

		if (maximumEnergy > 0) {
			for (let index = 0; index < energy.length; index += 1) {
				energy[index] /= maximumEnergy;
			}
		}

		if (maximumMass > 0) {
			for (let index = 0; index < mass.length; index += 1) {
				mass[index] /= maximumMass;
			}
		}

		this.points.geometry.setAttribute(
			"position",
			new THREE.BufferAttribute(positions, 3),
		);
		this.points.geometry.setAttribute(
			"aHeat",
			new THREE.BufferAttribute(heat, 1),
		);
		this.points.geometry.setAttribute(
			"aEnergy",
			new THREE.BufferAttribute(energy, 1),
		);
		this.points.geometry.setAttribute(
			"aMass",
			new THREE.BufferAttribute(mass, 1),
		);
		this.points.geometry.setAttribute(
			"aPhase",
			new THREE.BufferAttribute(phase, 1),
		);
		this.points.geometry.computeBoundingSphere();
		this.particles = particles;
	}

	setGridSpacing(spacing: number) {
		this.material.uniforms.uPointDiameter.value = spacing;
	}

	setProjectionScale(scale: number) {
		this.material.uniforms.uProjectionScale.value = scale;
	}

	particle(index: number) {
		return this.particles[index] ?? null;
	}

	dispose() {
		this.points.geometry.dispose();
		this.material.dispose();
	}
}
