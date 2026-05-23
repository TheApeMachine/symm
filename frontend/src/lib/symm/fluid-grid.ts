import type { FieldSnapshotEvent, FluidSymbolRow } from "#/lib/symm/events";

export const FLUID_GRID_SIZE = 32;
export const FLUID_HEIGHT_EMA_ALPHA = 0.35;

let smoothedHeights: number[][] | null = null;

export type FluidGrid = {
	heights: number[][];
	min: number;
	max: number;
	filledCells: number;
	outliers: FluidScaleSummary;
};

export type FluidScaleSummary = {
	clippedCount: number;
	clippedAt: number;
	rawMax: number;
	rawMaxSymbol?: string;
	displayMax: number;
};

function percentileRank(value: number, sorted: number[]): number {
	if (sorted.length === 0) return 0.5;
	let below = 0;
	for (const v of sorted) {
		if (v < value) below++;
	}
	return below / sorted.length;
}

function median(values: number[]): number {
	if (values.length === 0) return 0;
	const sorted = [...values].sort((a, b) => a - b);
	const mid = Math.floor(sorted.length / 2);
	if (sorted.length % 2 === 0) {
		return (sorted[mid - 1] + sorted[mid]) / 2;
	}
	return sorted[mid];
}

function quantile(sorted: number[], q: number): number {
	if (sorted.length === 0) return 0;
	const pos = (sorted.length - 1) * q;
	const base = Math.floor(pos);
	const rest = pos - base;
	const next = sorted[base + 1];
	if (next === undefined) return sorted[base];
	return sorted[base] + rest * (next - sorted[base]);
}

function binIndex(rank: number, size: number): number {
	const idx = Math.floor(rank * size);
	if (idx < 0) return 0;
	if (idx >= size) return size - 1;
	return idx;
}

function smoothEmptyCells(grid: number[][], fallback: number): number {
	const size = grid.length;
	let filled = 0;
	for (let z = 0; z < size; z++) {
		for (let x = 0; x < size; x++) {
			if (Number.isFinite(grid[z][x])) filled++;
		}
	}
	if (filled === 0) {
		for (let z = 0; z < size; z++) {
			for (let x = 0; x < size; x++) {
				grid[z][x] = fallback;
			}
		}
		return 0;
	}

	for (let pass = 0; pass < 3; pass++) {
		for (let z = 0; z < size; z++) {
			for (let x = 0; x < size; x++) {
				if (Number.isFinite(grid[z][x])) continue;
				let sum = 0;
				let count = 0;
				for (let dz = -1; dz <= 1; dz++) {
					for (let dx = -1; dx <= 1; dx++) {
						if (dz === 0 && dx === 0) continue;
						const nz = z + dz;
						const nx = x + dx;
						if (nz < 0 || nz >= size || nx < 0 || nx >= size) continue;
						const v = grid[nz][nx];
						if (!Number.isFinite(v)) continue;
						sum += v;
						count++;
					}
				}
				grid[z][x] = count > 0 ? sum / count : fallback;
			}
		}
	}
	return filled;
}

function displayRe(value: number, clippedAt: number): number {
	const clamped = Math.min(Math.max(value, 0), clippedAt);
	return Math.log1p(clamped);
}

function emaSmoothHeights(
	raw: number[][],
	alpha = FLUID_HEIGHT_EMA_ALPHA,
): number[][] {
	const size = raw.length;

	if (!smoothedHeights || smoothedHeights.length !== size) {
		smoothedHeights = raw.map((row) => [...row]);
		return smoothedHeights;
	}

	for (let z = 0; z < size; z++) {
		for (let x = 0; x < size; x++) {
			const next = raw[z][x];
			const prev = smoothedHeights[z][x];

			if (!Number.isFinite(next)) {
				continue;
			}

			if (!Number.isFinite(prev)) {
				smoothedHeights[z][x] = next;
				continue;
			}

			smoothedHeights[z][x] = alpha * next + (1 - alpha) * prev;
		}
	}

	return smoothedHeights;
}

export function resetFluidHeightSmoothing() {
	smoothedHeights = null;
}

/** Height proxy when tick Reynolds is near zero — uses 24h move and vol. */
function displayHeight(row: FluidSymbolRow, clippedAt: number): number {
	const re = displayRe(row.re, clippedAt);
	if (re > 1e-4) return re;
	const move = Math.log1p(Math.abs(row.change_pct));
	const vol = Math.log1p(Math.max(row.vol, 0));
	return move * 0.65 + vol * 0.35;
}

export function summarizeFluidScaling(
	rows: FluidSymbolRow[],
): FluidScaleSummary {
	const finiteRows = rows.filter((row) => Number.isFinite(row.re));
	if (finiteRows.length === 0) {
		return { clippedCount: 0, clippedAt: 0, rawMax: 0, displayMax: 0 };
	}

	const sortedRe = finiteRows.map((row) => row.re).sort((a, b) => a - b);
	const clippedAt = Math.max(quantile(sortedRe, 0.95), 0);
	let rawMax = Number.NEGATIVE_INFINITY;
	let rawMaxSymbol: string | undefined;
	let clippedCount = 0;

	for (const row of finiteRows) {
		if (row.re > rawMax) {
			rawMax = row.re;
			rawMaxSymbol = row.symbol;
		}
		if (row.re > clippedAt) clippedCount++;
	}

	return {
		clippedCount,
		clippedAt,
		rawMax,
		rawMaxSymbol,
		displayMax: displayRe(rawMax, clippedAt),
	};
}

/** Bin symbols by change% × vol rank; height = median Reynolds per cell. */
export function buildFluidGrid(
	rows: FluidSymbolRow[],
	size = FLUID_GRID_SIZE,
): FluidGrid {
	const heights = Array.from({ length: size }, () =>
		Array.from({ length: size }, () => Number.NaN),
	);
	const cells = Array.from({ length: size }, () =>
		Array.from({ length: size }, () => [] as number[]),
	);

	if (rows.length === 0) {
		return {
			heights,
			min: 0,
			max: 1,
			filledCells: 0,
			outliers: summarizeFluidScaling(rows),
		};
	}

	const finiteRows = rows.filter(
		(row) =>
			Number.isFinite(row.change_pct) &&
			Number.isFinite(row.vol) &&
			Number.isFinite(row.re),
	);
	const outliers = summarizeFluidScaling(finiteRows);
	if (finiteRows.length === 0) {
		return { heights, min: 0, max: 1, filledCells: 0, outliers };
	}

	const changes = finiteRows.map((r) => r.change_pct).sort((a, b) => a - b);
	const vols = finiteRows.map((r) => r.vol).sort((a, b) => a - b);
	const allDisplayRe = finiteRows.map((r) =>
		displayHeight(r, outliers.clippedAt),
	);
	const fallback = median(allDisplayRe);

	for (const row of finiteRows) {
		const x = binIndex(percentileRank(row.change_pct, changes), size);
		const z = binIndex(percentileRank(row.vol, vols), size);
		cells[z][x].push(displayHeight(row, outliers.clippedAt));
	}

	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;
	for (let z = 0; z < size; z++) {
		for (let x = 0; x < size; x++) {
			const values = cells[z][x];
			const y = values.length > 0 ? median(values) : Number.NaN;
			heights[z][x] = y;
			if (Number.isFinite(y)) {
				min = Math.min(min, y);
				max = Math.max(max, y);
			}
		}
	}

	const filledCells = smoothEmptyCells(heights, fallback);
	const smoothed = emaSmoothHeights(heights);

	for (let z = 0; z < size; z++) {
		for (let x = 0; x < size; x++) {
			heights[z][x] = smoothed[z][x];
		}
	}

	if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) {
		min = fallback - 0.5;
		max = fallback + 0.5;
	}

	return { heights, min, max, filledCells, outliers };
}

export function gridFromSnapshot(
	snapshot: FieldSnapshotEvent,
	size = FLUID_GRID_SIZE,
): FluidGrid {
	return buildFluidGrid(snapshot.symbols ?? [], size);
}
