/**
 * GPU texture generators for force-directed graph simulation
 *
 * These functions generate Float32Array data that gets uploaded to GPU textures.
 * The textures store node positions, velocities, attributes, and connectivity
 * information for the GPGPU force-directed layout algorithm.
 */

import * as THREE from "three";

/**
 * Calculate texture size as power of 2
 *
 * GPU textures work best with power-of-2 dimensions. This finds the smallest
 * power of 2 that can fit the required number of elements.
 */
export function indexTextureSize(dataArray: number[][]): number {
	const num = dataArray.length;
	let power = 1;
	while (power * power < num) {
		power *= 2;
	}
	return power / 2 > 1 ? power : 2;
}

/**
 * Calculate data texture size
 *
 * Since each pixel stores 4 floats (RGBA), we need to count all items
 * in the nested arrays.
 */
export function dataTextureSize(dataArray: number[][]): number {
	let count = 0;
	for (let i = 0; i < dataArray.length; i++) {
		count += dataArray[i]?.length ?? 0;
	}
	// Create a dummy array of the right length to calculate index size
	const dummyArray = new Array(Math.ceil(count / 4)).fill([]);
	return indexTextureSize(dummyArray);
}

/**
 * Count total items in nested arrays
 */
export function countDataArrayItems(dataArray: number[][]): number {
	let counter = 0;
	for (let i = 0; i < dataArray.length; i++) {
		counter += dataArray[i]?.length ?? 0;
	}
	return counter;
}

/**
 * Generate random position texture
 *
 * Creates initial random positions for nodes within a bounding cube.
 * Each pixel stores (x, y, z, w) where w=1 indicates valid node.
 */
export function generatePositionTexture(
	inputArray: number[][],
	textureSize: number,
	size: number,
): THREE.DataTexture {
	const bounds = size;
	const boundsHalf = bounds / 2;

	const textureArray = new Float32Array(textureSize * textureSize * 4);

	for (let i = 0; i < textureArray.length; i += 4) {
		if (i < inputArray.length * 4) {
			const x = Math.random() * bounds - boundsHalf;
			const y = Math.random() * bounds - boundsHalf;
			const z = Math.random() * bounds - boundsHalf;

			textureArray[i] = x;
			textureArray[i + 1] = y;
			textureArray[i + 2] = z;
			textureArray[i + 3] = 1.0;
		} else {
			// Fill remaining pixels with -1 (invalid marker)
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate zeroed position texture
 *
 * Used for layout targets - positions that nodes should move towards.
 */
export function generateZeroedPositionTexture(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);

	for (let i = 0; i < textureArray.length; i += 4) {
		if (i < inputArray.length * 4) {
			textureArray[i] = 0.0;
			textureArray[i + 1] = 0.0;
			textureArray[i + 2] = 0.0;
			textureArray[i + 3] = 0.0;
		} else {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate velocity texture
 *
 * Initial velocities are zero. The simulation will update these.
 */
export function generateVelocityTexture(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);

	for (let i = 0; i < textureArray.length; i += 4) {
		if (i < inputArray.length * 4) {
			textureArray[i] = 0.0;
			textureArray[i + 1] = 0.0;
			textureArray[i + 2] = 0.0;
			textureArray[i + 3] = 0.0;
		} else {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Node visual metrics from backend (normalized 0-1)
 */
export interface NodeMetrics {
	sizeNorm: number; // Based on parameter count
	brightnessNorm: number; // Based on centrality + tensor density
	weightMagNorm: number; // Based on weight magnitude
}

/**
 * Configuration for node attribute variation
 */
export interface NodeAttribConfig {
	/**
	 * Contrast multiplier for size/brightness differences.
	 * - 1.0 = normal (subtle differences)
	 * - 2.0 = doubled contrast (more obvious differences)
	 * - 3.0 = strong contrast (very noticeable)
	 * Default: 1.0
	 */
	contrast: number;
}

const DEFAULT_ATTRIB_CONFIG: NodeAttribConfig = {
	contrast: 1.0,
};

/**
 * Apply contrast to a normalized value [0, 1]
 *
 * Expands the distance from 0.5, clamped to [0, 1].
 *
 * contrast = 1.0: no change
 * contrast = 2.0: values are pushed 2x further from center
 * contrast = 3.0: values are pushed 3x further from center
 */
function applyContrast(value: number, contrast: number): number {
	if (contrast <= 1.0) return value;

	// Center around 0.5, multiply distance, re-center
	const centered = value - 0.5; // range [-0.5, 0.5]
	const expanded = centered * contrast; // multiply distance from center

	return Math.max(0, Math.min(1, 0.5 + expanded));
}

/**
 * Generate node attribute texture
 *
 * Stores per-node visual attributes:
 * - x: size (scaled from sizeNorm)
 * - y: current opacity (starts at initial, animated by shader)
 * - z: initial/base opacity (preserved for shader reference)
 * - w: unused
 *
 * Size range: [MIN_SIZE, MAX_SIZE] mapped from sizeNorm [0, 1]
 * Brightness range: [MIN_BRIGHTNESS, MAX_BRIGHTNESS] mapped from metrics
 */
export function generateNodeAttribTexture(
	inputArray: number[][],
	textureSize: number,
	nodeMetrics?: NodeMetrics[],
	config: NodeAttribConfig = DEFAULT_ATTRIB_CONFIG,
): THREE.DataTexture {
	// Size bounds - nodes shouldn't be too small or too large
	const MIN_SIZE = 100.0;
	const MAX_SIZE = 500.0;

	// Brightness bounds - keep visible but with contrast
	const MIN_BRIGHTNESS = 0.35;
	const MAX_BRIGHTNESS = 0.75;

	const { contrast } = config;

	const textureArray = new Float32Array(textureSize * textureSize * 4);

	for (let i = 0; i < textureArray.length; i += 4) {
		const idx = i / 4;
		if (idx < inputArray.length) {
			// Get metrics for this node (if available)
			const metrics = nodeMetrics?.[idx];

			let size: number;
			let brightness: number;

			if (metrics) {
				// Apply contrast to normalized values before mapping to ranges
				const sizeNormContrasted = applyContrast(metrics.sizeNorm, contrast);

				// Blend brightness from centrality/density and weight magnitude
				const blendedBrightness =
					0.7 * metrics.brightnessNorm + 0.3 * metrics.weightMagNorm;
				const brightnessContrasted = applyContrast(blendedBrightness, contrast);

				// Map to final ranges
				size = MIN_SIZE + sizeNormContrasted * (MAX_SIZE - MIN_SIZE);
				brightness =
					MIN_BRIGHTNESS +
					brightnessContrasted * (MAX_BRIGHTNESS - MIN_BRIGHTNESS);
			} else {
				// Fallback to defaults
				size = (MIN_SIZE + MAX_SIZE) / 2;
				brightness = (MIN_BRIGHTNESS + MAX_BRIGHTNESS) / 2;
			}

			textureArray[i] = size; // x: current size
			textureArray[i + 1] = brightness; // y: current opacity
			textureArray[i + 2] = brightness; // z: initial/base opacity (for shader reference)
			textureArray[i + 3] = 0.0; // w: unused
		} else {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate ID mappings texture
 *
 * Maps texture position to node ID for lookup operations.
 */
export function generateIdMappings(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);
	let counter = 0;

	for (let i = 0; i < textureArray.length; i += 4) {
		if (i < inputArray.length * 4) {
			textureArray[i] = counter;
			textureArray[i + 1] = 0;
			textureArray[i + 2] = 0;
			textureArray[i + 3] = 0;
		} else {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}
		counter++;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate indices texture
 *
 * Stores start/end indices for each node's edge list in the data texture.
 * Each pixel: (startPixel, startCoord, endPixel, endCoord)
 */
export function generateIndicesTexture(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);
	let currentPixel = 0;
	let currentCoord = 0;

	for (let i = 0; i < inputArray.length; i++) {
		const startPixel = currentPixel;
		const startCoord = currentCoord;

		for (let j = 0; j < (inputArray[i]?.length ?? 0); j++) {
			currentCoord++;
			if (currentCoord === 4) {
				currentPixel++;
				currentCoord = 0;
			}
		}

		textureArray[i * 4] = startPixel;
		textureArray[i * 4 + 1] = startCoord;
		textureArray[i * 4 + 2] = currentPixel;
		textureArray[i * 4 + 3] = currentCoord;
	}

	for (let i = inputArray.length * 4; i < textureArray.length; i++) {
		textureArray[i] = -1;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate data texture
 *
 * Packs edge data into a texture. Each float is a connected node ID.
 */
export function generateDataTexture(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);

	let currentIndex = 0;
	for (let i = 0; i < inputArray.length; i++) {
		for (let j = 0; j < (inputArray[i]?.length ?? 0); j++) {
			textureArray[currentIndex] = inputArray[i][j];
			currentIndex++;
		}
	}

	for (let i = currentIndex; i < textureArray.length; i++) {
		textureArray[i] = -1;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate epoch data texture
 *
 * Stores timestamp data for temporal visualization, offset from minimum epoch.
 */
export function generateEpochDataTexture(
	inputArray: number[][],
	textureSize: number,
	epochOffset: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);

	let currentIndex = 0;
	for (let i = 0; i < inputArray.length; i++) {
		for (let j = 0; j < (inputArray[i]?.length ?? 0); j++) {
			textureArray[currentIndex] = inputArray[i][j] - epochOffset;
			currentIndex++;
		}
	}

	for (let i = currentIndex; i < textureArray.length; i++) {
		textureArray[i] = -1;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

// Layout generators - create target positions for different layouts

/**
 * Generate circular layout positions
 */
export function generateCircularLayout(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const increase = (Math.PI * 2) / inputArray.length;
	let angle = 0;
	const radius = inputArray.length * 4 * 2;

	const textureArray = new Float32Array(textureSize * textureSize * 4);

	for (let i = 0; i < textureArray.length; i += 4) {
		if (i < inputArray.length * 4) {
			const x = radius * Math.cos(angle);
			const y = radius * Math.sin(angle);
			const z = 0;

			textureArray[i] = x;
			textureArray[i + 1] = y;
			textureArray[i + 2] = z;
			textureArray[i + 3] = 1.0;

			angle += increase;
		} else {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate spherical layout positions
 */
export function generateSphericalLayout(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const radius = inputArray.length * 4;
	const textureArray = new Float32Array(textureSize * textureSize * 4);
	const l = inputArray.length;

	for (let i = 0; i < l; i++) {
		const phi = Math.acos(-1 + (2 * i) / l);
		const theta = Math.sqrt(l * Math.PI) * phi;

		const x = radius * Math.cos(theta) * Math.sin(phi);
		const y = radius * Math.sin(theta) * Math.sin(phi);
		const z = radius * Math.cos(phi);

		textureArray[i * 4] = z;
		textureArray[i * 4 + 1] = y;
		textureArray[i * 4 + 2] = x;
		textureArray[i * 4 + 3] = 1.0;
	}

	for (let i = inputArray.length * 4; i < textureArray.length; i++) {
		textureArray[i] = -1;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate helix layout positions
 */
export function generateHelixLayout(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);
	const l = inputArray.length;

	for (let i = 0; i < l; i++) {
		const phi = i * 0.125 + Math.PI;

		const x = i * 15;
		const y = 500 * Math.sin(phi);
		const z = 500 * Math.cos(phi);

		textureArray[i * 4] = x;
		textureArray[i * 4 + 1] = y;
		textureArray[i * 4 + 2] = z;
		textureArray[i * 4 + 3] = 1.0;
	}

	for (let i = inputArray.length * 4; i < textureArray.length; i++) {
		textureArray[i] = -1;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate grid layout positions
 */
export function generateGridLayout(
	inputArray: number[][],
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);

	for (let i = 0; i < inputArray.length; i++) {
		const x = (i % 5) * 500 - 1000;
		const y = -(Math.floor(i / 5) % 5) * 500 + 1000;
		const z = Math.floor(i / 25) * 500 - 1000;

		textureArray[i * 4] = x;
		textureArray[i * 4 + 1] = y;
		textureArray[i * 4 + 2] = z;
		textureArray[i * 4 + 3] = 1.0;
	}

	for (let i = inputArray.length * 4; i < textureArray.length; i++) {
		textureArray[i] = -1;
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}

/**
 * Generate intensity texture
 *
 * Stores external node intensity values (0-1).
 */
export function generateIntensityTexture(
	intensities: number[] | undefined,
	textureSize: number,
): THREE.DataTexture {
	const textureArray = new Float32Array(textureSize * textureSize * 4);

	// Default to 0 intensity if not provided
	if (!intensities) {
		const texture = new THREE.DataTexture(
			textureArray,
			textureSize,
			textureSize,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	}

	for (let i = 0; i < textureArray.length; i += 4) {
		const nodeIndex = i / 4;
		if (nodeIndex < intensities.length) {
			const val = intensities[nodeIndex];
			textureArray[i] = val; // R
			textureArray[i + 1] = 0.0; // G
			textureArray[i + 2] = 0.0; // B
			textureArray[i + 3] = 0.0; // A
		} else {
			textureArray[i] = 0.0;
		}
	}

	const texture = new THREE.DataTexture(
		textureArray,
		textureSize,
		textureSize,
		THREE.RGBAFormat,
		THREE.FloatType,
	);
	texture.needsUpdate = true;
	return texture;
}
