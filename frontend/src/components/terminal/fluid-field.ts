import { clearCanvas, drawGrid } from "#/components/terminal/canvas";
import type { TerminalFluidParticle } from "#/components/terminal/fluid-particles";
import type { FluidFieldLayer } from "#/collections/terminal";
import { median } from "#/components/terminal/decision-format";
import { colormap } from "#/lib/colormap";

const DISPLAY_GAMMA = 0.55; // ponytail: fixed display gamma so midtones enter the mockup teal band; upgrade path is per-symbol histogram equalization from live lattice occupancy.

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
normalizeFluidLattice maps a field lattice onto the mockup colormap ramp using
min/max extent plus display gamma so the teal→gold band stays lit instead of
crushing the bulk of the lattice into near-black.
*/
export const normalizeFluidLattice = (
	matrix: number[][],
	contour: boolean,
): { normalized: number[][]; peak: number } => {
	const values = finiteValues(matrix);

	if (values.length === 0) {
		return { normalized: [], peak: 0 };
	}

	const { min, max } = matrixExtent(matrix);
	const span = Math.max(max - min, Number.EPSILON);
	let peak = 0;
	const normalized = matrix.map((row) =>
		row.map((value) => {
			let unit = Number.isFinite(value) ? (value - min) / span : 0;

			unit = Math.max(0, Math.min(1, unit));
			unit = unit > 0 ? unit ** DISPLAY_GAMMA : 0;

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
buildPilotFlowLattice splats particle velocities onto the price-time projection
so the overlay can fall back to observed particle motion when the solver does
not publish a guidance lattice.
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
resolvePilotDisplayLattice prefers |ψ|² as the primary cloud and falls back to
gas ρ only when the coherence projection is missing.
*/
export const resolvePilotDisplayLattice = (
	rho: number[][],
	psiMag2?: number[][],
): number[][] => {
	if (psiMag2 && isFluidFieldMatrix(psiMag2)) {
		return psiMag2;
	}

	if (isFluidFieldMatrix(rho)) {
		return rho;
	}

	return [];
};

/*
resolveFluidDisplayLattice selects one physical projection without mixing its
units with the other. Composite uses coherence as its base and draws gas as a
separately colored overlay.
*/
export const resolveFluidDisplayLattice = (
	rho: number[][],
	psiMag2: number[][],
	layer: FluidFieldLayer,
): number[][] => {
	if (layer === "Gas") {
		return isFluidFieldMatrix(rho) ? rho : [];
	}

	if (layer === "Coherence") {
		return isFluidFieldMatrix(psiMag2) ? psiMag2 : [];
	}

	return resolvePilotDisplayLattice(rho, psiMag2);
};

/*
compositeFieldLattice remains as a compatibility alias for the pilot-first lattice.
*/
export const compositeFieldLattice = (
	rho: number[][],
	psiMag2?: number[][],
): number[][] => resolvePilotDisplayLattice(rho, psiMag2);

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
meanGuidanceSpeed is the lattice-mean |v| of the published Bohm current so the
UI can read guidance from the same field the streaks paint when the scalar
reading is absent.
*/
export const meanGuidanceSpeed = (
	velX: number[][] | undefined,
	velZ: number[][] | undefined,
): number | null => {
	if (
		!velX ||
		!velZ ||
		!isFluidFieldMatrix(velX) ||
		!isFluidFieldMatrix(velZ)
	) {
		return null;
	}

	const rows = Math.min(velX.length, velZ.length);
	let total = 0;
	let count = 0;

	for (let row = 0; row < rows; row += 1) {
		const rowX = velX[row];
		const rowZ = velZ[row];
		const columns = Math.min(rowX?.length ?? 0, rowZ?.length ?? 0);

		for (let column = 0; column < columns; column += 1) {
			const speed = Math.hypot(rowX[column] ?? 0, rowZ[column] ?? 0);

			if (!Number.isFinite(speed)) {
				continue;
			}

			total += speed;
			count += 1;
		}
	}

	if (count === 0) {
		return null;
	}

	return total / count;
};

/*
drawGasDensityOverlay paints sparse ρ as a faint warm veil under the pilot-wave
cloud so gas mass remains readable without owning the frame.
*/
export const drawGasDensityOverlay = (
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
	pixels.fill(0);

	for (let row = 0; row < height; row += 1) {
		const sampleY = height <= 1 ? 0 : row / (height - 1);

		for (let column = 0; column < width; column += 1) {
			const sampleX = width <= 1 ? 0 : column / (width - 1);
			const unit = sampleBilinearLattice(normalized, sampleX, sampleY);

			if (unit <= 0) {
				continue;
			}

			const alpha = Math.floor(28 + unit * 70);
			const index = (row * width + column) * 4;

			pixels[index] = Math.floor(40 + unit * 90);
			pixels[index + 1] = Math.floor(28 + unit * 40);
			pixels[index + 2] = Math.floor(18 + unit * 20);
			pixels[index + 3] = alpha;
		}
	}

	const overlayCanvas = ensureFluidOverlayCanvas(width, height);

	if (overlayCanvas === null) {
		return;
	}

	const overlayContext = overlayCanvas.getContext("2d");

	if (overlayContext === null) {
		return;
	}

	overlayContext.putImageData(imageData, 0, 0);

	context.save();
	context.globalCompositeOperation = "screen";
	context.drawImage(overlayCanvas, 0, 0);
	context.restore();
};

/*
drawPilotWaveOverlay remains as a compatibility alias for the faint gas veil.
*/
export const drawPilotWaveOverlay = drawGasDensityOverlay;

/*
drawFluidFlowOverlay paints short guidance streaks masked by the display field so
directionality reads on top of |ψ|² without inventing motion the solver did not emit.
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
	// ponytail: fixed sparse-flow stride and field/speed cutoffs; upgrade path is lattice-density-aware sampling from observed particle occupancy.
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
			const field = lattice[row]?.[column] ?? 0;

			// ponytail: fixed field floor so streaks stay inside lit cloud cells like the tmp mockup; upgrade path is adaptive masking from live |ψ|² percentiles.
			if (field < 0.28) {
				continue;
			}

			const flowX = flow.flowX[row]?.[column] ?? 0;
			const flowY = flow.flowY[row]?.[column] ?? 0;
			const speed = Math.hypot(flowX, flowY);

			// ponytail: fixed speed floor for guidance streaks; upgrade path is lattice-speed percentile gating from observed guidance magnitudes.
			if (speed <= 0.08) {
				continue;
			}

			const directionX = flowX / speed;
			const directionY = flowY / speed;
			const centerX = (column + 0.5) * cellWidth;
			const centerY = (row + 0.5) * cellHeight;
			const streak =
				Math.min(cellWidth, cellHeight) * (0.55 + Math.min(speed, 1.2) * 0.45);

			context.strokeStyle = `rgba(255, 228, 180, ${0.12 + field * 0.22})`;
			context.lineWidth = 1;
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
	maximum: number;
	occupied: number;
};

export type TerminalFluidFieldDrawOptions = {
	particles?: TerminalFluidParticle[];
	pressureGradX?: number;
	pressureGradZ?: number;
	psiMag2?: number[][];
	layer?: FluidFieldLayer;
	guidanceVelX?: number[][];
	guidanceVelZ?: number[][];
};

type FluidPaintBuffer = {
	width: number;
	height: number;
	imageData: ImageData;
};

let fluidPaintBuffer: FluidPaintBuffer | null = null;

type FluidOverlayCanvas = {
	width: number;
	height: number;
	canvas: HTMLCanvasElement;
};

let fluidOverlayCanvas: FluidOverlayCanvas | null = null;

const ensureFluidOverlayCanvas = (
	width: number,
	height: number,
): HTMLCanvasElement | null => {
	if (width <= 0 || height <= 0 || typeof document === "undefined") {
		return null;
	}

	if (
		fluidOverlayCanvas === null ||
		fluidOverlayCanvas.width !== width ||
		fluidOverlayCanvas.height !== height
	) {
		const canvas = document.createElement("canvas");
		canvas.width = width;
		canvas.height = height;
		fluidOverlayCanvas = { width, height, canvas };
	}

	return fluidOverlayCanvas.canvas;
};

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
resampleFluidLattice projects one lattice onto the target rho grid so guidance
velocities can cover the full density field when backend frames disagree on size.
*/
export const resampleFluidLattice = (
	lattice: number[][],
	rows: number,
	columns: number,
): number[][] => {
	if (!isFluidFieldMatrix(lattice) || rows <= 0 || columns <= 0) {
		return [];
	}

	return Array.from({ length: rows }, (_, row) =>
		Array.from({ length: columns }, (_, column) => {
			const sampleX = columns <= 1 ? 0 : column / (columns - 1);
			const sampleY = rows <= 1 ? 0 : row / (rows - 1);

			return sampleBilinearLattice(lattice, sampleX, sampleY);
		}),
	);
};

const latticeMatchesShape = (
	lattice: number[][],
	rows: number,
	columns: number,
): boolean =>
	isFluidFieldMatrix(lattice) &&
	lattice.length === rows &&
	(lattice[0]?.length ?? 0) === columns;

const resolveGuidanceFlowLattice = (
	matrix: number[][],
	rows: number,
	columns: number,
	options: TerminalFluidFieldDrawOptions,
): FlowLattice => {
	const velX = options.guidanceVelX;
	const velZ = options.guidanceVelZ;

	if (velX && velZ && isFluidFieldMatrix(velX) && isFluidFieldMatrix(velZ)) {
		const alignedVelX = latticeMatchesShape(velX, rows, columns)
			? velX
			: resampleFluidLattice(velX, rows, columns);
		const alignedVelZ = latticeMatchesShape(velZ, rows, columns)
			? velZ
			: resampleFluidLattice(velZ, rows, columns);

		if (
			isFluidFieldMatrix(alignedVelX) &&
			isFluidFieldMatrix(alignedVelZ) &&
			latticeMatchesShape(alignedVelX, rows, columns) &&
			latticeMatchesShape(alignedVelZ, rows, columns)
		) {
			return normalizeFlowLattice(
				buildGuidanceFlowLattice(alignedVelX, alignedVelZ),
			);
		}
	}

	return normalizeFlowLattice(
		buildPilotFlowLattice(matrix, options.particles ?? []),
	);
};

/*
drawBlockFluidField paints one lattice cell per fillRect using the same chunky
heatmap contract as frontend/tmp (cell + 1px overlap, mockup colormap stops).
*/
export const drawBlockFluidField = (
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

	const cellWidth = width / columns;
	const cellHeight = height / rows;
	let peak = 0;

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			const unit = lattice[row]?.[column] ?? 0;

			peak = Math.max(peak, unit);

			const [red, green, blue] = colormap(unit);

			context.fillStyle = `rgb(${red | 0},${green | 0},${blue | 0})`;
			context.fillRect(
				column * cellWidth,
				row * cellHeight,
				cellWidth + 1,
				cellHeight + 1,
			);
		}
	}

	return peak;
};

/*
drawSmoothFluidField bilinearly upsamples a lattice onto the canvas when a
continuous read is preferred over the mockup block grid.
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
terminalFluidFieldStats reports raw occupancy and magnitude without presenting
display-normalized values as physical measurements.
*/
export const terminalFluidFieldStats = (
	matrix: number[][],
): TerminalFluidFieldStats => {
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;

	if (!isFluidFieldMatrix(matrix)) {
		return { columns: 0, rows: 0, maximum: 0, occupied: 0 };
	}

	const values = finiteValues(matrix);

	return {
		columns,
		rows,
		maximum: values.length > 0 ? Math.max(...values) : 0,
		occupied: values.filter((value) => value > 0).length,
	};
};

/*
drawFluidField paints the selected physical projection, adds gas as a distinct
overlay in composite mode, then lays the measured guidance current above it.
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

	const layer = options.layer ?? "Composite";
	const primary = resolveFluidDisplayLattice(
		matrix,
		options.psiMag2 ?? [],
		layer,
	);

	if (!isFluidFieldMatrix(primary)) {
		drawGrid(context, width, height);

		return { columns: 0, rows: 0, maximum: 0, occupied: 0 };
	}

	const { normalized } = normalizeMatrix(primary, contour);
	const rows = primary.length;
	const columns = primary[0]?.length ?? 0;

	drawBlockFluidField(context, width, height, normalized);

	if (
		layer === "Composite" &&
		isFluidFieldMatrix(matrix) &&
		isFluidFieldMatrix(options.psiMag2 ?? [])
	) {
		drawGasDensityOverlay(context, width, height, matrix);
	}

	const guidanceFlow = resolveGuidanceFlowLattice(
		primary,
		rows,
		columns,
		options,
	);

	drawFluidFlowOverlay(
		context,
		width,
		height,
		normalized,
		normalizeFlowLattice(guidanceFlow),
	);

	return terminalFluidFieldStats(primary);
};
