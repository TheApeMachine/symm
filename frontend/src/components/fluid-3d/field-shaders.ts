import { MAXIMUM_VOLUME_STEPS } from "./field-textures";

const frameUniforms = /* wgsl */ `
	struct Uniforms {
		viewProj: mat4x4<f32>,
		invViewProj: mat4x4<f32>,
		cameraPos: vec3<f32>,
		exposure: f32,
		grid: vec3<f32>,
		densityScale: f32,
		momentumScale: f32,
		energyScale: f32,
		waveScale: f32,
		showGas: f32,
		showWave: f32,
		sliceX: f32,
		sliceY: f32,
		sliceZ: f32,
	};
	@group(0) @binding(0) var<uniform> uniforms: Uniforms;
	@group(0) @binding(1) var fieldSampler: sampler;
	@group(0) @binding(2) var momRhoTexture: texture_3d<f32>;
	@group(0) @binding(3) var energyTexture: texture_3d<f32>;
	@group(0) @binding(4) var waveRealTexture: texture_3d<f32>;
	@group(0) @binding(5) var waveImagTexture: texture_3d<f32>;
`;

const fieldSampling = /* wgsl */ `
	const PI: f32 = 3.141592653589793;

	// Quantum domain coloring palette: vibrant iridescent phase wheel
	fn phaseColor(phase: f32) -> vec3<f32> {
		let p = phase;
		let r = 0.5 + 0.5 * cos(p);
		let g = 0.5 + 0.5 * cos(p - 2.0 * PI / 3.0);
		let b = 0.5 + 0.5 * cos(p - 4.0 * PI / 3.0);
		return pow(vec3<f32>(r, g, b), vec3<f32>(1.2)) * 1.5;
	}

	struct FluidSample {
		gasColor: vec3<f32>,
		gasExtinction: f32,
		waveGlow: vec3<f32>,
	};

	fn sampleFluid(coordinate: vec3<f32>) -> FluidSample {
		var field: FluidSample;
		field.gasColor = vec3<f32>(0.0);
		field.gasExtinction = 0.0;
		field.waveGlow = vec3<f32>(0.0);

		if (uniforms.showGas > 0.5) {
			let momRho = textureSampleLevel(momRhoTexture, fieldSampler, coordinate.zyx, 0.0);
			let density = abs(momRho.a) * uniforms.densityScale;
			let momMag = length(momRho.rgb) * uniforms.momentumScale;
			let energy = abs(textureSampleLevel(energyTexture, fieldSampler, coordinate.zyx, 0.0).r) * uniforms.energyScale;

			let rawSignal = max(density, momMag * 0.4);
			
			// Quadratic curve keeps gas whisper-thin and translucent instead of solid bricks
			let gasExtinction = pow(clamp(rawSignal * 0.35, 0.0, 1.0), 1.8) * 0.12;

			if (gasExtinction > 0.0001) {
				let darkAmber = vec3<f32>(0.40, 0.16, 0.03);
				let brightAmber = vec3<f32>(0.95, 0.55, 0.12);
				field.gasColor = mix(darkAmber, brightAmber, clamp(energy * 0.7, 0.0, 1.0));
				field.gasExtinction = gasExtinction;
			}
		}

		if (uniforms.showWave > 0.5) {
			let waveReal = textureSampleLevel(waveRealTexture, fieldSampler, coordinate.zyx, 0.0).r;
			let waveImag = textureSampleLevel(waveImagTexture, fieldSampler, coordinate.zyx, 0.0).r;
			let mag = length(vec2<f32>(waveReal, waveImag)) * uniforms.waveScale;

			if (mag > 0.001) {
				let wavePhase = select(0.0, atan2(waveImag, waveReal), abs(waveReal) > 1e-6 || abs(waveImag) > 1e-6);
				let caustic = pow(clamp(mag, 0.0, 1.0), 1.3) + 0.5 * pow(clamp(mag, 0.0, 1.0), 3.0);
				field.waveGlow = phaseColor(wavePhase) * caustic * 0.8;
			}
		}

		return field;
	}
`;

const vertexWorld = /* wgsl */ `
	struct VertexOut {
		@builtin(position) position: vec4<f32>,
		@location(0) world: vec3<f32>,
	};

	@vertex
	fn vs_main(@location(0) position: vec3<f32>) -> VertexOut {
		var output: VertexOut;
		output.world = position;
		output.position = uniforms.viewProj * vec4<f32>(position, 1.0);
		return output;
	}
`;

export const volumeShader = /* wgsl */ `
	${frameUniforms}
	${fieldSampling}
	${vertexWorld}

	const GAS_OPACITY: f32 = 0.45;
	const WAVE_EMISSION: f32 = 0.9;
	const MAX_STEPS: u32 = ${MAXIMUM_VOLUME_STEPS}u;

	fn intersectUnitBox(origin: vec3<f32>, direction: vec3<f32>) -> vec2<f32> {
		let inverseDirection = 1.0 / direction;
		let first = (vec3<f32>(0.0) - origin) * inverseDirection;
		let second = (vec3<f32>(1.0) - origin) * inverseDirection;
		let nearPlane = min(first, second);
		let farPlane = max(first, second);
		return vec2<f32>(
			max(max(nearPlane.x, nearPlane.y), nearPlane.z),
			min(min(farPlane.x, farPlane.y), farPlane.z)
		);
	}

	@fragment
	fn fs_main(input: VertexOut) -> @location(0) vec4<f32> {
		let rayDirection = normalize(input.world - uniforms.cameraPos);
		let intersection = intersectUnitBox(uniforms.cameraPos, rayDirection);
		let entrance = max(intersection.x, 0.0);

		if (intersection.y <= entrance) {
			discard;
		}

		let start = uniforms.cameraPos + rayDirection * entrance;
		let finish = uniforms.cameraPos + rayDirection * intersection.y;
		let sampleCount = max(ceil(length((finish - start) * uniforms.grid)), 1.0);
		let stepCount = min(u32(sampleCount), MAX_STEPS);
		let stepVector = (finish - start) / f32(stepCount);

		var accumulatedGas = vec3<f32>(0.0);
		var transmittance = 1.0;
		var waveEmission = vec3<f32>(0.0);

		for (var step = 0u; step < stepCount; step++) {
			let coordinate = start + (f32(step) + 0.5) * stepVector;
			let field = sampleFluid(coordinate);
			
			if (field.gasExtinction > 0.0) {
				let stepDensity = field.gasExtinction * uniforms.exposure * GAS_OPACITY;
				let stepTransmittance = exp(-stepDensity);
				let gasWeight = (1.0 - stepTransmittance) * transmittance;
				
				accumulatedGas += gasWeight * field.gasColor;
				transmittance *= stepTransmittance;
			}

			if (length(field.waveGlow) > 0.0) {
				waveEmission += transmittance * field.waveGlow * uniforms.exposure * WAVE_EMISSION * (1.0 / f32(stepCount)) * 14.0;
			}

			if (transmittance < 0.05) {
				break;
			}
		}

		let totalColor = accumulatedGas + waveEmission;
		let totalAlpha = clamp((1.0 - transmittance) + length(waveEmission) * 0.5, 0.0, 1.0);
		return vec4<f32>(totalColor, totalAlpha);
	}
`;

export const sliceShader = /* wgsl */ `
	${frameUniforms}
	${fieldSampling}
	${vertexWorld}

	@fragment
	fn fs_main(input: VertexOut) -> @location(0) vec4<f32> {
		let field = sampleFluid(clamp(input.world, vec3<f32>(0.0), vec3<f32>(1.0)));
		let gasAlpha = clamp(field.gasExtinction * uniforms.exposure * 1.5, 0.0, 1.0);
		let waveColor = field.waveGlow * uniforms.exposure * 0.5;
		return vec4<f32>(
			field.gasColor * gasAlpha + waveColor,
			clamp(max(gasAlpha, length(waveColor)), 0.0, 1.0)
		);
	}
`;

export const particleShader = /* wgsl */ `
	struct ParticleUniforms {
		viewProj: mat4x4<f32>,
		cameraRight: vec3<f32>,
		pointDiameter: f32,
		cameraUp: vec3<f32>,
		heatScale: f32,
		energyScale: f32,
		massScale: f32,
		amplitudeScale: f32,
		_pad0: f32,
	};
	@group(0) @binding(0) var<uniform> uniforms: ParticleUniforms;

	struct ParticleOut {
		@builtin(position) position: vec4<f32>,
		@location(0) uv: vec2<f32>,
		@location(1) heat: f32,
		@location(2) energy: f32,
		@location(3) amp: f32,
		@location(4) phase: f32,
	};

	@vertex
	fn vs_main(
		@location(0) corner: vec2<f32>,
		@location(1) particlePos: vec3<f32>,
		@location(2) mass: f32,
		@location(3) heat: f32,
		@location(4) energy: f32,
		@location(5) phase: f32,
		@location(6) amp: f32,
	) -> ParticleOut {
		let energyScale = 0.8 + 0.5 * clamp(energy, 0.0, 1.0);
		let massScale = 0.8 + 0.6 * clamp(mass * uniforms.massScale, 0.0, 1.0);
		let ampScale = 0.8 + 0.8 * clamp(amp * uniforms.amplitudeScale, 0.0, 1.0);
		let size = energyScale * massScale * ampScale * uniforms.pointDiameter;
		let world = particlePos
			+ uniforms.cameraRight * corner.x * size
			+ uniforms.cameraUp * corner.y * size;
		var output: ParticleOut;
		output.position = uniforms.viewProj * vec4<f32>(world, 1.0);
		output.uv = corner + vec2<f32>(0.5);
		output.heat = heat * uniforms.heatScale;
		output.energy = energy * uniforms.energyScale;
		output.amp = amp * uniforms.amplitudeScale;
		output.phase = phase;
		return output;
	}

	@fragment
	fn fs_main(input: ParticleOut) -> @location(0) vec4<f32> {
		let radius = length(input.uv - vec2<f32>(0.5));
		if (radius > 0.5) {
			discard;
		}

		let glow = smoothstep(0.5, 0.0, radius);
		let core = smoothstep(0.25, 0.0, radius);
		let heat = clamp(input.heat, 0.0, 1.0);

		// Thermal core: cold deep cyan -> warm orange -> hot incandescent white
		let cold = vec3<f32>(0.15, 0.38, 0.95);
		let warm = vec3<f32>(1.0, 0.42, 0.05);
		let hot = vec3<f32>(1.0, 0.96, 0.88);
		let thermoColor = select(
			mix(cold, warm, heat * 2.0),
			mix(warm, hot, (heat - 0.5) * 2.0),
			heat >= 0.5
		);

		// Kuramoto phase pulsing strobe ring
		let phaseStrobe = 0.5 + 0.5 * cos(input.phase);
		let pulseRing = smoothstep(0.48, 0.38, radius) * smoothstep(0.28, 0.38, radius) * phaseStrobe;

		// Wave amplitude halo
		let amp = clamp(input.amp, 0.0, 1.0);
		let ampHalo = glow * amp * 0.5;
		let haloColor = vec3<f32>(0.4, 0.85, 1.0);

		let brightness = mix(0.7, 1.4, pow(heat, 2.0)) * (core * 0.8 + glow * 0.25);
		return vec4<f32>(
			thermoColor * brightness + haloColor * (ampHalo + pulseRing * 1.5),
			glow * 0.9 + core * 0.1
		);
	}
`;

export const lineShader = /* wgsl */ `
	struct LineUniforms {
		viewProj: mat4x4<f32>,
	};
	@group(0) @binding(0) var<uniform> uniforms: LineUniforms;

	@vertex
	fn vs_main(@location(0) position: vec3<f32>) -> @builtin(position) vec4<f32> {
		return uniforms.viewProj * vec4<f32>(position, 1.0);
	}

	@fragment
	fn fs_main() -> @location(0) vec4<f32> {
		return vec4<f32>(0.33, 0.302, 0.263, 0.7);
	}
`;

export const currentShader = /* wgsl */ `
	struct CurrentUniforms {
		viewProj: mat4x4<f32>,
	};
	@group(0) @binding(0) var<uniform> uniforms: CurrentUniforms;

	struct CurrentOut {
		@builtin(position) position: vec4<f32>,
		@location(0) t: f32,
		@location(1) speed: f32,
	};

	@vertex
	fn vs_main(@location(0) position: vec3<f32>, @location(1) tailAndSpeed: vec2<f32>) -> CurrentOut {
		var output: CurrentOut;
		output.position = uniforms.viewProj * vec4<f32>(position, 1.0);
		output.t = tailAndSpeed.x;
		output.speed = tailAndSpeed.y;
		return output;
	}

	@fragment
	fn fs_main(input: CurrentOut) -> @location(0) vec4<f32> {
		let brightness = 0.3 + 0.7 * input.t;
		let slowColor = vec3<f32>(0.2, 0.7, 1.0);
		let fastColor = vec3<f32>(1.0, 0.85, 0.3);
		let streamColor = mix(slowColor, fastColor, clamp(input.speed * 2.0, 0.0, 1.0));
		return vec4<f32>(streamColor * brightness * 1.5, brightness * 0.8);
	}
`;
