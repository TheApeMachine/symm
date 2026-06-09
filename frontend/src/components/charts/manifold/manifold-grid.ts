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

	for (const carrier of carriers) {
		const cellX = clamp(carrier.cell_x, 0, grid.x - 1);
		const cellZ = clamp(carrier.cell_z, 0, grid.z - 1);
		const bump = carrier.role === "whale" ? carrier.heat : carrier.amplitude;
		const peak = yMin + clamp(bump, 0, 1) * (yMax - yMin) * 0.35;

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
				row[xIndex] = Math.max(row[xIndex] ?? yMin, peak * weight);
			}
		}
	}
};
