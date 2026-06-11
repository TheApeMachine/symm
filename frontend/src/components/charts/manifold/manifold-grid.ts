import {
	carrierDisplayWeight,
	normalizeCarrierWeights,
} from "#/components/charts/manifold/manifold-camera";

const clamp = (value: number, lower: number, upper: number): number =>
	Math.min(upper, Math.max(lower, value));

const MIN_CARRIER_PEAK_SCALE = 0.12;
const WEIGHT_PEAK_MULTIPLIER = 0.28;

const rowNumber = (row: unknown, key: string): number => {
	if (typeof row !== "object" || row === null) {
		return Number.NaN;
	}

	const value = (row as Record<string, unknown>)[key];

	return typeof value === "number" && Number.isFinite(value)
		? value
		: Number.NaN;
};

const rowString = (row: unknown, key: string): string => {
	if (typeof row !== "object" || row === null) {
		return "";
	}

	const value = (row as Record<string, unknown>)[key];

	return typeof value === "string" ? value : "";
};

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

const parseRho = (raw: unknown): number[][] | null => {
	if (!Array.isArray(raw) || raw.length === 0) {
		return null;
	}

	const rho = raw.map((row) => {
		if (!Array.isArray(row)) {
			return [];
		}

		return row.map((value) =>
			typeof value === "number" && Number.isFinite(value) ? value : 0,
		);
	});

	return rho.length > 0 && (rho[0]?.length ?? 0) > 0 ? rho : null;
};

const applyCarrierBumps = (
	heights: number[][],
	carriers: unknown[],
	gridX: number,
	gridZ: number,
	spacing: number,
	yMin: number,
	yMax: number,
) => {
	if (carriers.length === 0 || gridX <= 0 || gridZ <= 0 || spacing <= 0) {
		return;
	}

	const weights = normalizeCarrierWeights(carriers);

	for (const carrier of carriers) {
		const cellX = clamp(Math.round(rowNumber(carrier, "cell_x")), 0, gridX - 1);
		const cellZ = clamp(Math.round(rowNumber(carrier, "cell_z")), 0, gridZ - 1);
		const normalizedWeight = weights.get(rowString(carrier, "symbol")) ?? 0;
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

				if (xIndex < 0 || zIndex < 0 || xIndex >= gridX || zIndex >= gridZ) {
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

export const projectManifoldHeightmap = (
	frame: Record<string, unknown>,
	yMin: number,
	yMax: number,
) => {
	const rho = parseRho(frame.rho);

	if (rho === null) {
		return {
			heights: [] as number[][],
			gridX: 0,
			gridZ: 0,
			min: 0,
			max: 1,
		};
	}

	const gridZ = rho.length;
	const gridX = rho[0]?.length ?? 0;
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

	const gridRaw =
		typeof frame.grid === "object" && frame.grid !== null
			? (frame.grid as Record<string, unknown>)
			: {};
	const gridXCount = rowNumber(gridRaw, "x");
	const gridZCount = rowNumber(gridRaw, "z");
	const spacing = rowNumber(gridRaw, "spacing");
	const carriers = Array.isArray(frame.carriers) ? frame.carriers : [];

	applyCarrierBumps(
		normalized,
		carriers,
		Number.isFinite(gridXCount) ? gridXCount : gridX,
		Number.isFinite(gridZCount) ? gridZCount : gridZ,
		Number.isFinite(spacing) && spacing > 0 ? spacing : 1,
		yMin,
		yMax,
	);

	return {
		heights: normalized,
		gridX,
		gridZ,
		min,
		max,
	};
};

export { carrierDisplayWeight };
