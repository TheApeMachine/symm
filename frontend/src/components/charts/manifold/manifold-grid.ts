import {
	carrierDisplayWeight,
	normalizeCarrierWeights,
} from "#/components/charts/manifold/manifold-camera";
import type {
	ManifoldCarrierRow,
	ManifoldFieldSnapshot,
} from "#/components/charts/manifold/types";

export type ManifoldHeightmap = {
	heights: number[][];
	gridX: number;
	gridZ: number;
	min: number;
	max: number;
};

const clamp = (value: number, lower: number, upper: number): number =>
	Math.min(upper, Math.max(lower, value));

// Minimum carrier peak as a fraction of the chart y-range.
const MIN_CARRIER_PEAK_SCALE = 0.12;
// Scales weight-derived peak contribution before the minimum floor applies.
const WEIGHT_PEAK_MULTIPLIER = 0.28;

export const isDegenerateHeightmap = (heights: number[][]): boolean => {
	if (heights.length === 0 || (heights[0]?.length ?? 0) === 0) {
		return true;
	}

	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;

	for (const row of heights) {
		for (const value of row) {
			if (value < min) {
				min = value;
			}

			if (value > max) {
				max = value;
			}
		}
	}

	return !Number.isFinite(min) || !Number.isFinite(max) || max - min < 1e-4;
};

export const projectManifoldHeightmap = (
	frame: ManifoldFieldSnapshot,
	yMin: number,
	yMax: number,
): ManifoldHeightmap => {
	const rho = frame.rho;
	const gridZ = rho.length;
	const gridX = gridZ > 0 ? (rho[0]?.length ?? 0) : 0;
	const heights = rho.map((row) =>
		row.map((value) => (Number.isFinite(value) ? value : 0)),
	);

	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;

	for (const row of heights) {
		for (const value of row) {
			if (value < min) {
				min = value;
			}

			if (value > max) {
				max = value;
			}
		}
	}

	if (!Number.isFinite(min) || !Number.isFinite(max) || max <= min) {
		min = 0;
		max = 1;
	}

	const span = max - min;
	const normalized = heights.map((row) =>
		row.map((value) => yMin + ((value - min) / span) * (yMax - yMin)),
	);

	applyCarrierBumps(normalized, frame.carriers, frame.grid, yMin, yMax);

	return {
		heights: normalized,
		gridX,
		gridZ,
		min,
		max,
	};
};

const applyCarrierBumps = (
	heights: number[][],
	carriers: ManifoldCarrierRow[],
	grid: ManifoldFieldSnapshot["grid"],
	yMin: number,
	yMax: number,
) => {
	if (carriers.length === 0 || grid.x <= 0 || grid.z <= 0) {
		return;
	}

	const weights = normalizeCarrierWeights(carriers);

	for (const carrier of carriers) {
		const cellX = clamp(carrier.cell_x, 0, grid.x - 1);
		const cellZ = clamp(carrier.cell_z, 0, grid.z - 1);
		const normalizedWeight = weights.get(carrier.symbol) ?? 0;
		const peak =
			yMin +
			normalizedWeight *
				(yMax - yMin) *
				Math.max(
					MIN_CARRIER_PEAK_SCALE,
					WEIGHT_PEAK_MULTIPLIER * normalizedWeight,
				);

		for (let zOffset = -1; zOffset <= 1; zOffset += 1) {
			for (let xOffset = -1; xOffset <= 1; xOffset += 1) {
				const xIndex = cellX + xOffset;
				const zIndex = cellZ + zOffset;

				if (xIndex < 0 || zIndex < 0 || xIndex >= grid.x || zIndex >= grid.z) {
					continue;
				}

				const row = heights[zIndex];

				if (!row) {
					continue;
				}

				const weight = 1 / (1 + Math.abs(xOffset) + Math.abs(zOffset));
				row[xIndex] = Math.max(
					row[xIndex] ?? yMin,
					yMin + (peak - yMin) * weight,
				);
			}
		}
	}
};

export { carrierDisplayWeight };
