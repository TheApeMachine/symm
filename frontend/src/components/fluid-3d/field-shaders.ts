import { MAXIMUM_VOLUME_STEPS } from "./field-textures";

export const fluidFieldVertexShader = /* glsl */ `
	out vec3 vWorldPosition;

	void main() {
		vec4 worldPosition = modelMatrix * vec4(position, 1.0);
		vWorldPosition = worldPosition.xyz;
		gl_Position = projectionMatrix * viewMatrix * worldPosition;
	}
`;

const fieldSampling = /* glsl */ `
	uniform highp sampler3D uDensity;
	uniform highp sampler3D uMomentum;
	uniform highp sampler3D uInternalEnergy;
	uniform highp sampler3D uWaveReal;
	uniform highp sampler3D uWaveImaginary;
	uniform float uDensityScale;
	uniform float uMomentumScale;
	uniform float uEnergyScale;
	uniform float uWaveScale;
	uniform bool uShowGas;
	uniform bool uShowWave;

	const float PI = 3.141592653589793;

	vec3 phaseColor(float phase) {
		vec3 phaseOffsets = vec3(0.0, 2.0 * PI / 3.0, 4.0 * PI / 3.0);
		return 0.5 + 0.5 * cos(phase + phaseOffsets);
	}

	// Fields are mostly near-zero with a few sharp peaks after linear
	// max-normalization; a power curve lifts faint structure into visibility
	// as soft haze instead of only the peak cells ever reading as opaque.
	float compress(float normalized) {
		return pow(clamp(normalized, 0.0, 1.0), 0.35);
	}

	// Gas renders as colored fog (density-driven extinction, hot/cold by
	// internal energy). Wave renders as a separate additive glow so the two
	// phenomena stay visually distinct and independently toggleable.
	void sampleFluid(
		vec3 coordinate,
		out vec3 gasColor,
		out float gasExtinction,
		out vec3 waveGlow
	) {
		vec3 textureCoordinate = coordinate.zyx;
		float density = abs(texture(uDensity, textureCoordinate).r) * uDensityScale;
		vec3 momentum = texture(uMomentum, textureCoordinate).rgb;
		float momentumMagnitude = length(momentum) * uMomentumScale;
		float energy = abs(texture(uInternalEnergy, textureCoordinate).r) * uEnergyScale;
		float waveReal = texture(uWaveReal, textureCoordinate).r;
		float waveImaginary = texture(uWaveImaginary, textureCoordinate).r;
		float waveMagnitude =
			(waveReal * waveReal + waveImaginary * waveImaginary) * uWaveScale;

		float gasSignal = compress(max(density, momentumMagnitude));
		float waveSignal = compress(waveMagnitude);

		vec3 darkBrown = vec3(0.14, 0.09, 0.05);
		vec3 lightBrown = vec3(0.62, 0.44, 0.28);
		gasColor = mix(darkBrown, lightBrown, clamp(compress(energy), 0.0, 1.0));
		gasExtinction = uShowGas ? gasSignal : 0.0;

		vec3 waveColor = phaseColor(atan(waveImaginary, waveReal));
		waveGlow = uShowWave ? waveColor * waveSignal : vec3(0.0);
	}
`;

export const fluidVolumeFragmentShader = /* glsl */ `
	precision highp float;
	precision highp sampler3D;
	in vec3 vWorldPosition;
	out vec4 outputColor;
	uniform vec3 uGrid;
	uniform float uExposure;
	// Gas fog is dampened and wave glow is boosted relative to the shared
	// exposure control so gas reads as a light haze rather than dominating
	// the wave field.
	const float GAS_OPACITY = 0.35;
	const float WAVE_BRIGHTNESS = 2.5;
	${fieldSampling}

	vec2 intersectUnitBox(vec3 origin, vec3 direction) {
		vec3 inverseDirection = 1.0 / direction;
		vec3 first = (vec3(0.0) - origin) * inverseDirection;
		vec3 second = (vec3(1.0) - origin) * inverseDirection;
		vec3 nearPlane = min(first, second);
		vec3 farPlane = max(first, second);
		return vec2(
			max(max(nearPlane.x, nearPlane.y), nearPlane.z),
			min(min(farPlane.x, farPlane.y), farPlane.z)
		);
	}

	void main() {
		vec3 rayDirection = normalize(vWorldPosition - cameraPosition);
		vec2 intersection = intersectUnitBox(cameraPosition, rayDirection);
		float entrance = max(intersection.x, 0.0);

		if (intersection.y <= entrance) {
			discard;
		}

		vec3 start = cameraPosition + rayDirection * entrance;
		vec3 finish = cameraPosition + rayDirection * intersection.y;
		float sampleCount = max(ceil(length((finish - start) * uGrid)), 1.0);
		vec3 stepVector = (finish - start) / sampleCount;
		vec3 accumulated = vec3(0.0);
		float accumulatedAlpha = 0.0;
		vec3 waveAccumulated = vec3(0.0);

		for (int step = 0; step < ${MAXIMUM_VOLUME_STEPS}; step += 1) {
			if (float(step) >= sampleCount || accumulatedAlpha >= 1.0) {
				break;
			}

			vec3 coordinate = start + (float(step) + 0.5) * stepVector;
			vec3 gasColor;
			float gasExtinction;
			vec3 waveGlow;
			sampleFluid(coordinate, gasColor, gasExtinction, waveGlow);
			float alpha = 1.0 - exp(-gasExtinction * uExposure * GAS_OPACITY);
			accumulated += (1.0 - accumulatedAlpha) * alpha * gasColor;
			accumulatedAlpha += (1.0 - accumulatedAlpha) * alpha;
			// Wave glow accumulates independently of gas opacity so a foggy
			// ray does not extinguish the wave field behind it.
			waveAccumulated += waveGlow * uExposure * WAVE_BRIGHTNESS;
		}

		float outputAlpha = clamp(max(accumulatedAlpha, length(waveAccumulated)), 0.0, 1.0);
		outputColor = vec4(accumulated + waveAccumulated, outputAlpha);
	}
`;

export const fluidSliceFragmentShader = /* glsl */ `
	precision highp float;
	precision highp sampler3D;
	in vec3 vWorldPosition;
	out vec4 outputColor;
	uniform float uExposure;
	const float GAS_OPACITY = 0.35;
	const float WAVE_BRIGHTNESS = 2.5;
	${fieldSampling}

	void main() {
		vec3 gasColor;
		float gasExtinction;
		vec3 waveGlow;
		sampleFluid(clamp(vWorldPosition, 0.0, 1.0), gasColor, gasExtinction, waveGlow);
		float gasAlpha = clamp(gasExtinction * uExposure * GAS_OPACITY, 0.0, 1.0);
		vec3 waveColor = waveGlow * uExposure * WAVE_BRIGHTNESS;
		vec3 color = gasColor * gasAlpha + waveColor;
		outputColor = vec4(color, clamp(max(gasAlpha, length(waveColor)), 0.0, 1.0));
	}
`;
