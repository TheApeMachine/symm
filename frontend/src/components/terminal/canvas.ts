export const TERMINAL_COLORS = {
	background: "#0e0c0a",
	surface: "#17140f",
	line: "#2b251e",
	lineStrong: "#3a342b",
	foreground: "#f4efe5",
	muted: "#938a7e",
	amber: "#e8a33d",
	cyan: "#7fbacb",
	green: "#9cc06e",
	red: "#d5786a",
} as const;

export const resizeCanvas = (
	canvas: HTMLCanvasElement,
): CanvasRenderingContext2D | null => {
	const width = Math.max(1, canvas.clientWidth);
	const height = Math.max(1, canvas.clientHeight);
	const ratio = window.devicePixelRatio || 1;

	if (
		canvas.width !== Math.floor(width * ratio) ||
		canvas.height !== Math.floor(height * ratio)
	) {
		canvas.width = Math.floor(width * ratio);
		canvas.height = Math.floor(height * ratio);
	}

	const context = canvas.getContext("2d");

	if (context === null) {
		return null;
	}

	context.setTransform(ratio, 0, 0, ratio, 0, 0);

	return context;
};

export const clearCanvas = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => {
	context.clearRect(0, 0, width, height);
	context.fillStyle = TERMINAL_COLORS.background;
	context.fillRect(0, 0, width, height);
};

export const clamp01 = (value: number): number =>
	Math.min(1, Math.max(0, value));

const hexToRgb = (hex: string): [number, number, number] => [
	parseInt(hex.slice(1, 3), 16),
	parseInt(hex.slice(3, 5), 16),
	parseInt(hex.slice(5, 7), 16),
];

const lerp = (left: number, right: number, amount: number): number =>
	left + (right - left) * amount;

const mix = (left: string, right: string, amount: number): string => {
	const a = hexToRgb(left);
	const b = hexToRgb(right);

	return `rgb(${Math.round(lerp(a[0], b[0], amount))},${Math.round(lerp(a[1], b[1], amount))},${Math.round(lerp(a[2], b[2], amount))})`;
};

export const heatColor = (value: number): string => {
	const normalized = clamp01(value);

	if (normalized < 0.45) {
		return mix(TERMINAL_COLORS.background, "#1b2232", normalized / 0.45);
	}

	if (normalized < 0.7) {
		return mix("#1b2232", TERMINAL_COLORS.cyan, (normalized - 0.45) / 0.25);
	}

	if (normalized < 0.88) {
		return mix(
			TERMINAL_COLORS.cyan,
			TERMINAL_COLORS.amber,
			(normalized - 0.7) / 0.18,
		);
	}

	return mix(
		TERMINAL_COLORS.amber,
		TERMINAL_COLORS.foreground,
		(normalized - 0.88) / 0.12,
	);
};

export const drawGrid = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	padding = 14,
) => {
	context.strokeStyle = TERMINAL_COLORS.line;
	context.lineWidth = 1;

	for (let index = 0; index <= 4; index += 1) {
		const y = padding + index * ((height - padding * 2) / 4);
		context.beginPath();
		context.moveTo(padding, y);
		context.lineTo(width - padding, y);
		context.stroke();
	}
};

export const drawPolyline = (
	context: CanvasRenderingContext2D,
	points: Array<{ x: number; y: number }>,
	color: string,
	dashed = false,
) => {
	if (points.length < 2) {
		return;
	}

	context.strokeStyle = color;
	context.lineWidth = 1.8;
	context.setLineDash(dashed ? [4, 3] : []);
	context.beginPath();

	for (let index = 0; index < points.length; index += 1) {
		const point = points[index];

		if (index === 0) {
			context.moveTo(point.x, point.y);
			continue;
		}

		context.lineTo(point.x, point.y);
	}

	context.stroke();
	context.setLineDash([]);
};

const matrixExtent = (matrix: number[][]) => {
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

export const drawMatrix = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	matrix: number[][],
	contour = false,
) => {
	clearCanvas(context, width, height);

	if (matrix.length === 0 || (matrix[0]?.length ?? 0) === 0) {
		drawGrid(context, width, height);
		return;
	}

	const { min, max } = matrixExtent(matrix);
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;
	const cellWidth = width / columns;
	const cellHeight = height / rows;

	for (let rowIndex = 0; rowIndex < rows; rowIndex += 1) {
		for (let columnIndex = 0; columnIndex < columns; columnIndex += 1) {
			const value = matrix[rowIndex]?.[columnIndex] ?? min;
			let normalized = (value - min) / (max - min);

			if (contour) {
				normalized = Math.floor(normalized / 0.12) * 0.12;
			}

			context.fillStyle = heatColor(normalized);
			context.fillRect(
				columnIndex * cellWidth,
				rowIndex * cellHeight,
				cellWidth + 1,
				cellHeight + 1,
			);
		}
	}
};
