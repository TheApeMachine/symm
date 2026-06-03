export const cellSmoothRadius = (
	baseRadius: number,
	turbulence: number,
): number => {
	if (!Number.isFinite(baseRadius) || baseRadius <= 0) {
		return 0;
	}

	if (!Number.isFinite(turbulence) || turbulence <= 0) {
		return baseRadius;
	}

	return Math.max(0, Math.round(baseRadius / (1 + turbulence)));
};

export const anomalySNRForActivity = (
	activity: number,
	clippedAt: number,
): number => {
	if (!Number.isFinite(activity) || activity <= 0 || clippedAt <= 0) {
		return 0;
	}

	if (activity <= clippedAt) {
		return 0;
	}

	return (activity - clippedAt) / clippedAt;
};

function gaussianKernel(radius: number): number[][] {
	const sigma = Math.max(radius / 2, 0.5);
	const size = radius * 2 + 1;
	const kernel: number[][] = [];
	let weightSum = 0;

	for (let rowIndex = 0; rowIndex < size; rowIndex++) {
		const row: number[] = [];
		const deltaZ = rowIndex - radius;

		for (let colIndex = 0; colIndex < size; colIndex++) {
			const deltaX = colIndex - radius;
			const weight = Math.exp(
				-(deltaX * deltaX + deltaZ * deltaZ) / (2 * sigma * sigma),
			);

			row.push(weight);
			weightSum += weight;
		}

		kernel.push(row);
	}

	for (let rowIndex = 0; rowIndex < size; rowIndex++) {
		for (let colIndex = 0; colIndex < size; colIndex++) {
			kernel[rowIndex][colIndex] /= weightSum;
		}
	}

	return kernel;
}

function smoothCell(
	heightmap: number[][],
	zIndex: number,
	xIndex: number,
	radius: number,
): number {
	const zSize = heightmap.length;
	const xSize = heightmap[0]?.length ?? 0;
	const kernel = gaussianKernel(radius);
	let value = 0;
	let weightSum = 0;

	for (let kernelZ = 0; kernelZ < kernel.length; kernelZ++) {
		for (let kernelX = 0; kernelX < kernel[kernelZ].length; kernelX++) {
			const sampleZ = Math.min(
				Math.max(zIndex + kernelZ - radius, 0),
				zSize - 1,
			);
			const sampleX = Math.min(
				Math.max(xIndex + kernelX - radius, 0),
				xSize - 1,
			);
			const weight = kernel[kernelZ][kernelX];

			value += heightmap[sampleZ][sampleX] * weight;
			weightSum += weight;
		}
	}

	return weightSum > 0 ? value / weightSum : heightmap[zIndex][xIndex];
}

/** Spatial Gaussian smooth with per-cell radius inversely scaled by turbulence. */
export const smoothHeightmapSpatialAdaptive = (
	heightmap: number[][],
	turbulence: number[][],
	baseRadius: number,
): number[][] => {
	const zSize = heightmap.length;
	const xSize = heightmap[0]?.length ?? 0;

	if (zSize === 0 || xSize === 0) {
		return heightmap.map((row) => [...row]);
	}

	const smoothed = Array.from({ length: zSize }, () =>
		Array.from({ length: xSize }, () => 0),
	);

	for (let zIndex = 0; zIndex < zSize; zIndex++) {
		for (let xIndex = 0; xIndex < xSize; xIndex++) {
			const turb = turbulence[zIndex]?.[xIndex] ?? 0;
			const radius = cellSmoothRadius(baseRadius, turb);

			if (radius <= 0) {
				smoothed[zIndex][xIndex] = heightmap[zIndex][xIndex];
				continue;
			}

			smoothed[zIndex][xIndex] = smoothCell(
				heightmap,
				zIndex,
				xIndex,
				radius,
			);
		}
	}

	return smoothed;
};

export const emaSmoothHeightsVolumeAware = (
	raw: number[][],
	volumes: number[][],
	previous: number[][] | null,
	alpha = 0.35,
): number[][] => {
	const size = raw.length;

	if (!previous || previous.length !== size) {
		return raw.map((row) => [...row]);
	}

	const smoothed = previous.map((row) => [...row]);
	const flatVolumes = volumes.flat().filter((value) => value > 0);
	const medianVolume =
		flatVolumes.length > 0
			? flatVolumes.sort((left, right) => left - right)[
					Math.floor(flatVolumes.length / 2)
				]
			: 0;

	for (let zIndex = 0; zIndex < size; zIndex++) {
		for (let xIndex = 0; xIndex < size; xIndex++) {
			const next = raw[zIndex][xIndex];
			const prev = smoothed[zIndex][xIndex];

			if (!Number.isFinite(next)) {
				continue;
			}

			if (!Number.isFinite(prev)) {
				smoothed[zIndex][xIndex] = next;
				continue;
			}

			const volume = volumes[zIndex]?.[xIndex] ?? 0;
			const volumeScale =
				medianVolume > 0 && volume > 0 ? volume / medianVolume : 0;
			const cellAlpha = alpha / (1 + volumeScale);

			smoothed[zIndex][xIndex] = cellAlpha * next + (1 - cellAlpha) * prev;
		}
	}

	return smoothed;
};

export const peakAnomalyIntensity = (anomalySNR: number[][]): number => {
	let peak = 0;

	for (const row of anomalySNR) {
		for (const value of row) {
			if (Number.isFinite(value)) {
				peak = Math.max(peak, value);
			}
		}
	}

	return peak;
};

export const visualStressFromAnomaly = (
	baseHighlight: number,
	baseHardness: number,
	anomalySNR: number[][],
): { highlight: number; cellHardnessFactor: number } => {
	const peak = peakAnomalyIntensity(anomalySNR);
	const scale = Math.min(2, peak);

	return {
		highlight: baseHighlight * (1 + scale),
		cellHardnessFactor: baseHardness * (1 + scale * 0.5),
	};
};
