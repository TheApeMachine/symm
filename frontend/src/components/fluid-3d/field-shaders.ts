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
	const float NUMERIC_EPSILON = 0.000001;

	vec3 phaseColor(float phase) {
		vec3 phaseOffsets = vec3(0.0, 2.0 * PI / 3.0, 4.0 * PI / 3.0);
		return 0.5 + 0.5 * cos(phase + phaseOffsets);
	}

	void sampleFluid(vec3 coordinate, out vec3 color, out float extinction) {
		vec3 textureCoordinate = coordinate.zyx;
		float density = abs(texture(uDensity, textureCoordinate).r) * uDensityScale;
		vec3 momentum = texture(uMomentum, textureCoordinate).rgb;
		float momentumMagnitude = length(momentum) * uMomentumScale;
		float energy = abs(texture(uInternalEnergy, textureCoordinate).r) * uEnergyScale;
		float waveReal = texture(uWaveReal, textureCoordinate).r;
		float waveImaginary = texture(uWaveImaginary, textureCoordinate).r;
		float waveMagnitude =
			(waveReal * waveReal + waveImaginary * waveImaginary) * uWaveScale;
		float gasWeight = uShowGas ? clamp(density, 0.0, 1.0) : 0.0;
		float waveWeight = uShowWave ? clamp(waveMagnitude, 0.0, 1.0) : 0.0;
		vec3 cold = vec3(0.08, 0.34, 1.0);
		vec3 hot = vec3(1.0, 0.38, 0.05);
		vec3 gasColor = mix(cold, hot, clamp(energy, 0.0, 1.0));
		gasColor *= max(density, momentumMagnitude);
		vec3 waveColor = phaseColor(atan(waveImaginary, waveReal));
		extinction = gasWeight + waveWeight;
		color = (gasColor * gasWeight + waveColor * waveWeight) /
			max(extinction, NUMERIC_EPSILON);
	}
`;

export const fluidVolumeFragmentShader = /* glsl */ `
	precision highp float;
	precision highp sampler3D;
	in vec3 vWorldPosition;
	out vec4 outputColor;
	uniform vec3 uGrid;
	uniform float uExposure;
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

		for (int step = 0; step < ${MAXIMUM_VOLUME_STEPS}; step += 1) {
			if (float(step) >= sampleCount || accumulatedAlpha >= 1.0) {
				break;
			}

			vec3 coordinate = start + (float(step) + 0.5) * stepVector;
			vec3 color;
			float extinction;
			sampleFluid(coordinate, color, extinction);
			float alpha = 1.0 - exp(-extinction * uExposure / sampleCount);
			accumulated += (1.0 - accumulatedAlpha) * alpha * color;
			accumulatedAlpha += (1.0 - accumulatedAlpha) * alpha;
		}

		outputColor = vec4(accumulated, accumulatedAlpha);
	}
`;

export const fluidSliceFragmentShader = /* glsl */ `
	precision highp float;
	precision highp sampler3D;
	in vec3 vWorldPosition;
	out vec4 outputColor;
	uniform float uExposure;
	${fieldSampling}

	void main() {
		vec3 color;
		float extinction;
		sampleFluid(clamp(vWorldPosition, 0.0, 1.0), color, extinction);
		outputColor = vec4(color, clamp(extinction * uExposure, 0.0, 1.0));
	}
`;
