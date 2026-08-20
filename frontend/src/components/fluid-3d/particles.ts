import * as THREE from "three";
import type { FluidParticleFrame } from "./wire";

const particleVertexShader = /* glsl */ `
	attribute float aHeat;
	attribute float aEnergy;
	attribute float aMass;
	attribute float aPhase;
	uniform float uProjectionScale;
	uniform float uPointDiameter;
	uniform float uHeatScale;
	uniform float uEnergyScale;
	uniform float uMassScale;
	varying float vHeat;
	varying float vEnergy;
	varying float vPhase;

	void main() {
		vec4 viewPosition = modelViewMatrix * vec4(position, 1.0);
		vHeat = aHeat * uHeatScale;
		vEnergy = aEnergy * uEnergyScale;
		vPhase = aPhase;
		// Modulate point size cleanly within unit box scale
		float energyScale = 0.8 + 0.5 * clamp(aEnergy, 0.0, 1.0);
		float massScale = 0.8 + 0.6 * clamp(aMass * uMassScale, 0.0, 1.0);
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

/*
FluidParticles is one GPU point buffer and one draw call for the complete
resident particle selection. No particle owns a mesh or expanded geometry.
*/
export class FluidParticles {
	readonly points: THREE.Points;
	private frame: FluidParticleFrame | null = null;
	private readonly material: THREE.ShaderMaterial;
	private interleaved: THREE.InterleavedBuffer | null = null;

	constructor() {
		this.material = new THREE.ShaderMaterial({
			uniforms: {
				uProjectionScale: { value: 1 },
				uPointDiameter: { value: 1 / 64 },
				uHeatScale: { value: 0 },
				uEnergyScale: { value: 0 },
				uMassScale: { value: 0 },
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

	update(frame: FluidParticleFrame) {
		if (
			this.interleaved !== null &&
			this.interleaved.stride === frame.stride &&
			this.interleaved.array.length === frame.values.length
		) {
			this.interleaved.array = frame.values;
			this.interleaved.needsUpdate = true;
			this.points.geometry.setDrawRange(0, frame.count);
			this.updateScales(frame);
			this.frame = frame;
			return;
		}

		const interleaved = new THREE.InterleavedBuffer(frame.values, frame.stride);
		interleaved.setUsage(THREE.StreamDrawUsage);
		const geometry = new THREE.BufferGeometry();
		geometry.setAttribute(
			"position",
			new THREE.InterleavedBufferAttribute(interleaved, 3, 0, false),
		);
		geometry.setAttribute(
			"aMass",
			new THREE.InterleavedBufferAttribute(interleaved, 1, 6, false),
		);
		geometry.setAttribute(
			"aHeat",
			new THREE.InterleavedBufferAttribute(interleaved, 1, 7, false),
		);
		geometry.setAttribute(
			"aEnergy",
			new THREE.InterleavedBufferAttribute(interleaved, 1, 8, false),
		);
		geometry.setAttribute(
			"aPhase",
			new THREE.InterleavedBufferAttribute(interleaved, 1, 9, false),
		);
		geometry.setDrawRange(0, frame.count);
		geometry.boundingSphere = new THREE.Sphere(
			new THREE.Vector3(0.5, 0.5, 0.5),
			Math.sqrt(3) / 2,
		);
		const previous = this.points.geometry;
		this.points.geometry = geometry;
		this.interleaved = interleaved;
		previous.dispose();
		this.updateScales(frame);
		this.frame = frame;
	}

	private updateScales(frame: FluidParticleFrame) {
		this.material.uniforms.uHeatScale.value = frame.heatScale;
		this.material.uniforms.uEnergyScale.value = frame.energyScale;
		this.material.uniforms.uMassScale.value = frame.massScale;
	}

	setGridSpacing(spacing: number) {
		this.material.uniforms.uPointDiameter.value = spacing;
	}

	setProjectionScale(scale: number) {
		this.material.uniforms.uProjectionScale.value = scale;
	}

	particle(index: number) {
		return this.frame?.particle(index) ?? null;
	}

	dispose() {
		this.points.geometry.dispose();
		this.material.dispose();
	}
}
