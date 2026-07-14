import {
	clearCanvas,
	drawGrid,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { mad, median } from "#/components/terminal/decision-format";
import { colormap } from "#/lib/colormap";

const OUTLIER_MAD = 1.5;

export type TerminalFluidFieldStats = {
	columns: number;
	rows: number;
	peak: number;
	outliers: number;
};

export type TerminalFluidParticle = {
	source: string;
	role: string;
	cellX: number;
	cellY: number;
	cellZ: number;
	phase: number;
	omega: number;
	amplitude: number;
	heat: number;
	velX: number;
	velY: number;
	velZ: number;
	speed: number;
};

const matrixExtent = (matrix: number[][]): { min: number; max: number } => {
	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;

	for (const row of matrix) {
		for (const value of row) {
			if (!Number.isFinite(value)) {
				continue;
			}

			min = Math.min(min, value);
			max = Math.max(max, value);
		}
	}

	if (!Number.isFinite(min) || !Number.isFinite(max) || max <= min) {
		return { min: 0, max: 1 };
	}

	return { min, max };
};

const normalizeMatrix = (
	matrix: number[][],
	contour: boolean,
): { normalized: number[][]; peak: number } => {
	const { min, max } = matrixExtent(matrix);
	const span = max - min || 1;
	let peak = 0;
	const normalized = matrix.map((row) =>
		row.map((value) => {
			let unit = (value - min) / span;

			unit = Math.max(0, Math.min(1, unit));

			if (contour) {
				unit = Math.floor(unit / 0.12) * 0.12;
			}

			peak = Math.max(peak, unit);

			return unit;
		}),
	);

	return { normalized, peak };
};

const outlierCount = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const center = median(values);
	const dispersion = mad(values);
	const threshold = center + OUTLIER_MAD * dispersion;

	return values.filter((value) => value > threshold).length;
};

type FluidPaintBuffer = {
	width: number;
	height: number;
	imageData: ImageData;
};

let fluidPaintBuffer: FluidPaintBuffer | null = null;

const ensureFluidPaintBuffer = (
	width: number,
	height: number,
): ImageData | null => {
	if (width <= 0 || height <= 0) {
		return null;
	}

	if (
		fluidPaintBuffer === null ||
		fluidPaintBuffer.width !== width ||
		fluidPaintBuffer.height !== height
	) {
		fluidPaintBuffer = {
			width,
			height,
			imageData: new ImageData(width, height),
		};
	}

	return fluidPaintBuffer.imageData;
};

/*
sampleBilinearLattice reads one normalized coordinate from a rho projection.
*/
export const sampleBilinearLattice = (
	lattice: number[][],
	x: number,
	y: number,
): number => {
	const rows = lattice.length;
	const columns = lattice[0]?.length ?? 0;

	if (rows === 0 || columns === 0) {
		return 0;
	}

	const clampedX = Math.max(0, Math.min(1, x));
	const clampedY = Math.max(0, Math.min(1, y));
	const sampleX = clampedX * (columns - 1);
	const sampleY = clampedY * (rows - 1);
	const column0 = Math.floor(sampleX);
	const row0 = Math.floor(sampleY);
	const column1 = Math.min(column0 + 1, columns - 1);
	const row1 = Math.min(row0 + 1, rows - 1);
	const deltaX = sampleX - column0;
	const deltaY = sampleY - row0;
	const northwest = lattice[row0]?.[column0] ?? 0;
	const northeast = lattice[row0]?.[column1] ?? 0;
	const southwest = lattice[row1]?.[column0] ?? 0;
	const southeast = lattice[row1]?.[column1] ?? 0;
	const north = northwest + (northeast - northwest) * deltaX;
	const south = southwest + (southeast - southwest) * deltaX;

	return north + (south - north) * deltaY;
};

/*
drawSmoothFluidField bilinearly upsamples rho onto the canvas so the field reads
as continuous clouds instead of discrete lattice blocks.
*/
export const drawSmoothFluidField = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	lattice: number[][],
): number => {
	const rows = lattice.length;
	const columns = lattice[0]?.length ?? 0;

	if (rows === 0 || columns === 0 || width <= 0 || height <= 0) {
		return 0;
	}

	const imageData = ensureFluidPaintBuffer(width, height);

	if (imageData === null) {
		return 0;
	}

	const pixels = imageData.data;
	let peak = 0;

	for (let row = 0; row < height; row += 1) {
		const sampleY = height <= 1 ? 0 : row / (height - 1);

		for (let column = 0; column < width; column += 1) {
			const sampleX = width <= 1 ? 0 : column / (width - 1);
			const unit = sampleBilinearLattice(lattice, sampleX, sampleY);

			peak = Math.max(peak, unit);

			const [red, green, blue] = colormap(unit);
			const index = (row * width + column) * 4;

			pixels[index] = red | 0;
			pixels[index + 1] = green | 0;
			pixels[index + 2] = blue | 0;
			pixels[index + 3] = 255;
		}
	}

	context.putImageData(imageData, 0, 0);

	return peak;
};

/*
isFluidFieldMatrix reports whether a rho projection is a paintable 2D lattice.
*/
export const isFluidFieldMatrix = (matrix: number[][]): boolean => {
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;

	return rows >= 2 && columns >= 2;
};

/*
terminalFluidFieldStats derives the terminal field readout from one rho projection.
*/
export const terminalFluidFieldStats = (
	matrix: number[][],
	contour = false,
): TerminalFluidFieldStats => {
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;

	if (!isFluidFieldMatrix(matrix)) {
		return { columns: 0, rows: 0, peak: 0, outliers: 0 };
	}

	const { normalized, peak } = normalizeMatrix(matrix, contour);

	return {
		columns,
		rows,
		peak,
		outliers: outlierCount(normalized.flat()),
	};
};

/*
drawFluidField paints the terminal density field from one rho projection.
*/
export const drawFluidField = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	matrix: number[][],
	contour = false,
): TerminalFluidFieldStats => {
	clearCanvas(context, width, height);

	if (!isFluidFieldMatrix(matrix)) {
		drawGrid(context, width, height);

		return { columns: 0, rows: 0, peak: 0, outliers: 0 };
	}

	const { normalized, peak } = normalizeMatrix(matrix, contour);

	drawSmoothFluidField(context, width, height, normalized);

	return {
		columns: matrix[0]?.length ?? 0,
		rows: matrix.length,
		peak,
		outliers: outlierCount(normalized.flat()),
	};
};

/*
drawFluidWhaleCarriers paints whale-carrier markers over the density field.
*/
export const drawFluidWhaleCarriers = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	particles: TerminalFluidParticle[],
	columns: number,
	rows: number,
) => {
	for (const particle of particles) {
		const role = particle.role.toLowerCase();

		if (!role.includes("whale")) {
			continue;
		}

		const x = (particle.cellX / Math.max(columns - 1, 1)) * width;
		const y = (particle.cellZ / Math.max(rows - 1, 1)) * height;
		const glow = context.createRadialGradient(x, y, 0, x, y, 22);

		glow.addColorStop(0, TERMINAL_COLORS.amber);
		glow.addColorStop(1, "rgba(0,0,0,0)");
		context.fillStyle = glow;
		context.beginPath();
		context.arc(x, y, 22, 0, Math.PI * 2);
		context.fill();

		context.fillStyle = "#fff";
		context.beginPath();
		context.arc(x, y, 2.4, 0, Math.PI * 2);
		context.fill();
	}
};
