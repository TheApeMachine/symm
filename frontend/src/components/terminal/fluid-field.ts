import { clearCanvas, drawGrid } from "#/components/terminal/canvas";
import { mad, median } from "#/components/terminal/decision-format";
import { colormap } from "#/lib/colormap";

const OUTLIER_MAD = 1.5;

const finiteValues = (matrix: number[][]): number[] =>
	matrix.flat().filter((value) => Number.isFinite(value));

const matrixExtent = (matrix: number[][]): { min: number; max: number } => {
	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;

	for (const value of finiteValues(matrix)) {
		min = Math.min(min, value);
		max = Math.max(max, value);
	}

	if (!Number.isFinite(min) || !Number.isFinite(max) || max <= min) {
		return { min: 0, max: 1 };
	}

	return { min, max };
};

/*
normalizeFluidLattice maps raw rho onto a display lattice using median/MAD
contrast so sparse gas deposits read as continuous clouds instead of faint
streaks against a crushed background.
*/
export const normalizeFluidLattice = (
	matrix: number[][],
	contour: boolean,
): { normalized: number[][]; peak: number } => {
	const values = finiteValues(matrix);

	if (values.length === 0) {
		return { normalized: [], peak: 0 };
	}

	const center = median(values);
	const dispersion = mad(values);
	const { min, max } = matrixExtent(matrix);
	const span =
		dispersion > 0
			? dispersion * (1 + OUTLIER_MAD)
			: Math.max(max - min, Number.EPSILON);
	const floor = dispersion > 0 ? center - dispersion : min;
	let peak = 0;
	const normalized = matrix.map((row) =>
		row.map((value) => {
			let unit = (value - floor) / span;

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

const normalizeMatrix = (
	matrix: number[][],
	contour: boolean,
): { normalized: number[][]; peak: number } =>
	normalizeFluidLattice(matrix, contour);

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

type FlowLattice = {
	flowX: number[][];
	flowY: number[][];
};

const emptyFlowLattice = (rows: number, columns: number): FlowLattice => ({
	flowX: Array.from({ length: rows }, () =>
		Array.from({ length: columns }, () => 0),
	),
	flowY: Array.from({ length: rows }, () =>
		Array.from({ length: columns }, () => 0),
	),
});

/*
buildPilotFlowLattice splats oscillator guidance velocities onto the price-time
projection so the overlay reflects the pilot-wave current particles actually
ride, not a decorative animation.
*/
export const buildPilotFlowLattice = (
	lattice: number[][],
	particles: TerminalFluidParticle[],
): FlowLattice => {
	const rows = lattice.length;
	const columns = lattice[0]?.length ?? 0;

	if (rows === 0 || columns === 0) {
		return emptyFlowLattice(0, 0);
	}

	const flow = emptyFlowLattice(rows, columns);
	const weight = Array.from({ length: rows }, () =>
		Array.from({ length: columns }, () => 0),
	);

	for (const particle of particles) {
		const column = Math.round(particle.cellX);
		const row = Math.round(particle.cellZ);

		if (row < 0 || row >= rows || column < 0 || column >= columns) {
			continue;
		}

		const mass = Math.max(particle.amplitude, 0);

		if (mass <= 0) {
			continue;
		}

		flow.flowX[row][column] += particle.velX * mass;
		flow.flowY[row][column] += particle.velZ * mass;
		weight[row][column] += mass;
	}

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			const cellWeight = weight[row][column];

			if (cellWeight <= 0) {
				continue;
			}

			flow.flowX[row][column] /= cellWeight;
			flow.flowY[row][column] /= cellWeight;
		}
	}

	return flow;
};

/*
buildPressureFlowLattice turns the bulk gas-pressure gradient into a uniform
direction field so the canvas still shows transport bias when local oscillator
velocities are sparse.
*/
export const buildPressureFlowLattice = (
	rows: number,
	columns: number,
	pressureGradX: number,
	pressureGradZ: number,
): FlowLattice => {
	const flow = emptyFlowLattice(rows, columns);
	const speed = Math.hypot(pressureGradX, pressureGradZ);

	if (speed <= 0) {
		return flow;
	}

	const flowX = -pressureGradX / speed;
	const flowY = -pressureGradZ / speed;

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			flow.flowX[row][column] = flowX;
			flow.flowY[row][column] = flowY;
		}
	}

	return flow;
};

/*
mergeFlowLattices prefers local pilot-wave velocities and only falls back to
bulk pressure transport where the oscillator field is empty.
*/
export const mergeFlowLattices = (
	primary: FlowLattice,
	fallback: FlowLattice,
): FlowLattice => {
	const rows = primary.flowY.length;
	const columns = primary.flowX[0]?.length ?? 0;
	const merged = emptyFlowLattice(rows, columns);

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			const localSpeed = Math.hypot(
				primary.flowX[row][column],
				primary.flowY[row][column],
			);

			if (localSpeed > 0) {
				merged.flowX[row][column] = primary.flowX[row][column];
				merged.flowY[row][column] = primary.flowY[row][column];
				continue;
			}

			merged.flowX[row][column] = fallback.flowX[row][column] ?? 0;
			merged.flowY[row][column] = fallback.flowY[row][column] ?? 0;
		}
	}

	return merged;
};

/*
compositeFieldLattice blends gas density and pilot-wave magnitude so the canvas
shows continuous field structure instead of a sparse PIC deposit stripe.
*/
export const compositeFieldLattice = (
	rho: number[][],
	psiMag2?: number[][],
): number[][] => {
	if (!isFluidFieldMatrix(rho)) {
		return [];
	}

	if (!psiMag2 || !isFluidFieldMatrix(psiMag2)) {
		return rho;
	}

	return rho.map((row, rowIndex) =>
		row.map((value, columnIndex) => {
			const psi = psiMag2[rowIndex]?.[columnIndex] ?? 0;

			return Math.max(value, psi * 0.85);
		}),
	);
};

/*
normalizeFlowLattice scales guidance velocities by their observed percentile so
flow streaks remain visible without inventing directionality.
*/
export const normalizeFlowLattice = (flow: FlowLattice): FlowLattice => {
	const rows = flow.flowY.length;
	const columns = flow.flowX[0]?.length ?? 0;
	const speeds: number[] = [];

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			const speed = Math.hypot(
				flow.flowX[row]?.[column] ?? 0,
				flow.flowY[row]?.[column] ?? 0,
			);

			if (speed > 0) {
				speeds.push(speed);
			}
		}
	}

	if (speeds.length === 0) {
		return flow;
	}

	const scale = Math.max(median(speeds), Number.EPSILON);
	const normalized = emptyFlowLattice(rows, columns);

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			normalized.flowX[row][column] = (flow.flowX[row]?.[column] ?? 0) / scale;
			normalized.flowY[row][column] = (flow.flowY[row]?.[column] ?? 0) / scale;
		}
	}

	return normalized;
};

/*
buildGuidanceFlowLattice maps the projected Bohm guidance velocity field onto the
display lattice so directionality comes from the coherence field itself.
*/
export const buildGuidanceFlowLattice = (
	velX: number[][],
	velZ: number[][],
): FlowLattice => ({
	flowX: velX,
	flowY: velZ,
});

/*
drawPilotWaveOverlay paints |psi|^2 as a cool coherence veil over the gas field.
*/
export const drawPilotWaveOverlay = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	lattice: number[][],
) => {
	const rows = lattice.length;
	const columns = lattice[0]?.length ?? 0;

	if (rows === 0 || columns === 0 || width <= 0 || height <= 0) {
		return;
	}

	const { normalized } = normalizeFluidLattice(lattice, false);
	const imageData = ensureFluidPaintBuffer(width, height);

	if (imageData === null) {
		return;
	}

	const pixels = imageData.data;

	for (let row = 0; row < height; row += 1) {
		const sampleY = height <= 1 ? 0 : row / (height - 1);

		for (let column = 0; column < width; column += 1) {
			const sampleX = width <= 1 ? 0 : column / (width - 1);
			const unit = sampleBilinearLattice(normalized, sampleX, sampleY);

			if (unit <= 0) {
				continue;
			}

			const alpha = Math.floor(90 + unit * 120);
			const index = (row * width + column) * 4;

			pixels[index] = Math.floor(24 + unit * 70);
			pixels[index + 1] = Math.floor(72 + unit * 110);
			pixels[index + 2] = Math.floor(120 + unit * 90);
			pixels[index + 3] = alpha;
		}
	}

	context.save();
	context.globalCompositeOperation = "screen";
	context.putImageData(imageData, 0, 0);
	context.restore();
};

/*
drawFluidFlowOverlay paints short guidance streaks masked by rho so directionality
reads on top of the gas heatmap without inventing motion the solver did not emit.
*/
export const drawFluidFlowOverlay = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	lattice: number[][],
	flow: FlowLattice,
) => {
	const rows = lattice.length;
	const columns = lattice[0]?.length ?? 0;

	if (rows === 0 || columns === 0 || width <= 0 || height <= 0) {
		return;
	}

	const cellWidth = width / columns;
	const cellHeight = height / rows;
	const strideX = Math.max(1, Math.floor(columns / 24));
	const strideY = Math.max(1, Math.floor(rows / 16));

	context.save();
	context.lineCap = "round";

	for (let row = Math.floor(strideY / 2); row < rows; row += strideY) {
		for (
			let column = Math.floor(strideX / 2);
			column < columns;
			column += strideX
		) {
			const rho = lattice[row]?.[column] ?? 0;

			if (rho < 0.03) {
				continue;
			}

			const flowX = flow.flowX[row]?.[column] ?? 0;
			const flowY = flow.flowY[row]?.[column] ?? 0;
			const speed = Math.hypot(flowX, flowY);

			if (speed <= 0.04) {
				continue;
			}

			const directionX = flowX / speed;
			const directionY = flowY / speed;
			const centerX = (column + 0.5) * cellWidth;
			const centerY = (row + 0.5) * cellHeight;
			const streak =
				Math.min(cellWidth, cellHeight) * (0.85 + Math.min(speed, 1.2) * 0.75);

			context.strokeStyle = `rgba(255, 228, 180, ${0.22 + rho * 0.42})`;
			context.lineWidth = 1.15;
			context.beginPath();
			context.moveTo(
				centerX - directionX * streak * 0.5,
				centerY - directionY * streak * 0.5,
			);
			context.lineTo(
				centerX + directionX * streak * 0.5,
				centerY + directionY * streak * 0.5,
			);
			context.stroke();
		}
	}

	context.restore();
};

export type TerminalFluidFieldStats = {
	columns: number;
	rows: number;
	peak: number;
	outliers: number;
};

export type TerminalFluidFieldDrawOptions = {
	particles?: TerminalFluidParticle[];
	pressureGradX?: number;
	pressureGradZ?: number;
	psiMag2?: number[][];
	guidanceVelX?: number[][];
	guidanceVelZ?: number[][];
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

	const { peak } = normalizeMatrix(matrix, contour);

	return {
		columns,
		rows,
		peak,
		outliers: outlierCount(finiteValues(matrix)),
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
	options: TerminalFluidFieldDrawOptions = {},
): TerminalFluidFieldStats => {
	clearCanvas(context, width, height);

	if (!isFluidFieldMatrix(matrix)) {
		drawGrid(context, width, height);

		return { columns: 0, rows: 0, peak: 0, outliers: 0 };
	}

	const { normalized, peak } = normalizeMatrix(
		compositeFieldLattice(matrix, options.psiMag2),
		contour,
	);
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;

	drawSmoothFluidField(context, width, height, normalized);

	if (options.psiMag2 && isFluidFieldMatrix(options.psiMag2)) {
		drawPilotWaveOverlay(context, width, height, options.psiMag2);
	}

	const guidanceFlow =
		options.guidanceVelX &&
		options.guidanceVelZ &&
		isFluidFieldMatrix(options.guidanceVelX) &&
		isFluidFieldMatrix(options.guidanceVelZ)
			? normalizeFlowLattice(
					buildGuidanceFlowLattice(options.guidanceVelX, options.guidanceVelZ),
				)
			: normalizeFlowLattice(
					buildPilotFlowLattice(matrix, options.particles ?? []),
				);
	const pressureFlow = buildPressureFlowLattice(
		rows,
		columns,
		options.pressureGradX ?? 0,
		options.pressureGradZ ?? 0,
	);

	drawFluidFlowOverlay(
		context,
		width,
		height,
		normalized,
		mergeFlowLattices(guidanceFlow, pressureFlow),
	);

	return {
		columns,
		rows,
		peak,
		outliers: outlierCount(finiteValues(matrix)),
	};
};

/*
drawFluidWhaleCarriers paints compact carrier markers with local velocity hints.
*/
export const drawFluidWhaleCarriers = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	particles: TerminalFluidParticle[],
	columns: number,
	rows: number,
) => {
	const carriers = particles
		.filter((particle) => particle.role.toLowerCase().includes("whale"))
		.sort((left, right) => right.amplitude - left.amplitude)
		.slice(0, 16);
	const cellWidth = width / Math.max(columns, 1);
	const cellHeight = height / Math.max(rows, 1);
	const markerRadius = Math.max(3, Math.min(cellWidth, cellHeight) * 0.22);

	for (const particle of carriers) {
		const x = (particle.cellX / Math.max(columns - 1, 1)) * width;
		const y = (particle.cellZ / Math.max(rows - 1, 1)) * height;
		const glow = context.createRadialGradient(
			x,
			y,
			0,
			x,
			y,
			markerRadius * 2.4,
		);

		glow.addColorStop(0, "rgba(255, 196, 96, 0.9)");
		glow.addColorStop(1, "rgba(0,0,0,0)");
		context.fillStyle = glow;
		context.beginPath();
		context.arc(x, y, markerRadius * 2.4, 0, Math.PI * 2);
		context.fill();

		context.fillStyle = "#fff";
		context.beginPath();
		context.arc(x, y, Math.max(1.4, markerRadius * 0.35), 0, Math.PI * 2);
		context.fill();

		const speed = Math.hypot(particle.velX, particle.velZ);

		if (speed <= 0) {
			continue;
		}

		const directionX = particle.velX / speed;
		const directionY = particle.velZ / speed;
		const tick = markerRadius * 1.8;

		context.strokeStyle = "rgba(255, 228, 180, 0.75)";
		context.lineWidth = 1;
		context.beginPath();
		context.moveTo(x, y);
		context.lineTo(x + directionX * tick, y + directionY * tick);
		context.stroke();
	}
};
