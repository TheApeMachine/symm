import * as THREE from "three";
import type { FluidParticle } from "./wire";

const particleVertexShader = /* glsl */ `
	attribute float aHeat;
	attribute float aEnergy;
	attribute float aMass;
	uniform float uProjectionScale;
	uniform float uPointDiameter;
	varying float vHeat;
	varying float vEnergy;

	void main() {
		vec4 viewPosition = modelViewMatrix * vec4(position, 1.0);
		vHeat = aHeat;
		vEnergy = aEnergy;
		// Energy modulates point size, not brightness, so it stays visually
		// distinct from the heat-driven color temperature. Mass grows the
		// point independently, so heavier particles read as visibly larger.
		float energyScale = 0.6 + 0.8 * clamp(vEnergy, 0.0, 1.0);
		float massScale = 0.7 + 1.6 * clamp(aMass, 0.0, 1.0);
		gl_PointSize = max(
			1.0,
			energyScale * massScale * uPointDiameter * uProjectionScale /
				max(-viewPosition.z, 0.000001)
		);
		gl_Position = projectionMatrix * viewPosition;
	}
`;

const particleFragmentShader = /* glsl */ `
	varying float vHeat;
	varying float vEnergy;

	void main() {
		float radius = length(gl_PointCoord - vec2(0.5));

		if (radius > 0.5) {
			discard;
		}

		float glow = smoothstep(0.5, 0.0, radius);
		float heat = clamp(vHeat, 0.0, 1.0);
		vec3 cold = vec3(0.05, 0.2, 0.65);
		vec3 mid = vec3(1.0, 0.42, 0.06);
		vec3 white = vec3(1.0, 0.97, 0.85);
		vec3 color = heat < 0.5
			? mix(cold, mid, heat * 2.0)
			: mix(mid, white, (heat - 0.5) * 2.0);
		// Brightness comes from heat and the point's glow falloff only, so a
		// high-energy but cold particle never reads as visually "hot".
		float brightness = mix(0.35, 1.2, heat) * glow;
		// A thin energy ring at the point edge gives Energy its own cue
		// without touching color temperature or overall brightness.
		float energyRing = smoothstep(0.42, 0.5, radius) *
			smoothstep(0.28, 0.35, radius) * clamp(vEnergy, 0.0, 1.0);
		gl_FragColor = vec4(color * brightness + vec3(energyRing), glow);
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
