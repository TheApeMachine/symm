import {
	clearCanvas,
	drawGrid,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type { LatentPoint } from "#/components/terminal/xray-view";

/*
drawXrayWaiting clears an xray canvas and paints a muted waiting caption.
*/
export const drawXrayWaiting = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	message: string,
) => {
	clearCanvas(context, width, height);
	drawGrid(context, width, height);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "11px JetBrains Mono, monospace";
	context.fillText(message, 18, Math.max(52, height * 0.34));
};

/*
categoryColor picks a latent scatter color from regime text and focus state.
*/
export const categoryColor = (category: string, focus: boolean): string => {
	const normalized = category.toLowerCase();

	if (focus) {
		return TERMINAL_COLORS.amber;
	}

	if (normalized.includes("stress") || normalized.includes("turbulent")) {
		return TERMINAL_COLORS.red;
	}

	if (normalized.includes("flow") || normalized.includes("laminar")) {
		return TERMINAL_COLORS.green;
	}

	if (normalized.includes("coupling") || normalized.includes("equilibrium")) {
		return TERMINAL_COLORS.cyan;
	}

	return TERMINAL_COLORS.muted;
};

/*
latentRange returns a finite axis span so scatter projection never divides by zero.
*/
export const latentRange = (
	points: LatentPoint[],
	key: "x" | "y",
): { min: number; span: number } => {
	const values = points.map((point) => point[key]);
	const min = Math.min(...values);
	const max = Math.max(...values);
	const span = max - min;

	if (!Number.isFinite(min) || !Number.isFinite(span)) {
		return { min: 0, span: 1 };
	}

	if (span <= 0) {
		return { min: min - 0.5, span: 1 };
	}

	return { min, span };
};

/*
latentPointScreen maps a latent carrier into canvas coordinates for hit-testing.
*/
export const latentPointScreen = (
	point: LatentPoint,
	points: LatentPoint[],
	width: number,
	height: number,
	pad = 28,
) => {
	const xRange = latentRange(points, "x");
	const yRange = latentRange(points, "y");

	return {
		x: pad + ((point.x - xRange.min) / xRange.span) * (width - pad * 2),
		y:
			height -
			pad -
			((point.y - yRange.min) / yRange.span) * (height - pad * 2),
	};
};
