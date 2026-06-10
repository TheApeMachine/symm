export const manifoldCameraFrame = (
	gridX: number,
	gridZ: number,
	ySpan: number,
): {
	centerX: number;
	centerZ: number;
	yCenter: number;
	orbit: number;
	worldHeight: number;
} => {
	const centerX = (gridX - 1) / 8;
	const centerZ = (gridZ - 1) / 4;
	const span = Math.max(gridX, gridZ, 1);
	const orbit = span * 1.15;
	const safeYSpan = Math.max(ySpan, 0.25);
	const yCenter = safeYSpan / 2;

	return {
		centerX,
		centerZ,
		yCenter,
		orbit,
		worldHeight: Math.max(safeYSpan * span * 0.42, 6),
	};
};

export const manifoldHeightExtent = (
	heights: number[][],
): { min: number; max: number } => {
	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;

	for (const row of heights) {
		for (const value of row) {
			if (!Number.isFinite(value)) {
				continue;
			}

			if (value < min) {
				min = value;
			}

			if (value > max) {
				max = value;
			}
		}
	}

	if (!Number.isFinite(min) || !Number.isFinite(max)) {
		return { min: 0, max: 1 };
	}

	if (max <= min) {
		const pad = Math.max(Math.abs(min) * 0.1, 0.05);

		return { min: min - pad, max: max + pad };
	}

	return { min, max };
};

export const carrierDisplayWeight = (carrier: {
	role: string;
	amplitude: number;
	heat: number;
}): number => (carrier.role === "whale" ? carrier.heat : carrier.amplitude);

export const normalizeCarrierWeights = (
	carriers: Array<{
		role: string;
		amplitude: number;
		heat: number;
		symbol: string;
	}>,
): Map<string, number> => {
	let maxWeight = 0;

	for (const carrier of carriers) {
		const weight = carrierDisplayWeight(carrier);

		if (weight > maxWeight) {
			maxWeight = weight;
		}
	}

	if (maxWeight <= 0) {
		maxWeight = 1;
	}

	return new Map(
		carriers.map((carrier) => [
			carrier.symbol,
			carrierDisplayWeight(carrier) / maxWeight,
		]),
	);
};
