import {
	clearCanvas,
	drawGrid,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type { LatentPoint } from "#/components/terminal/xray-view";
import { requirePositive, requirePositiveLength } from "#/lib/domain";

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
latentRange returns the finite axis span for scatter projection.
A zero or non-finite span is an undefined scale, not a unit fallback.
*/
export const latentRange = (
	points: LatentPoint[],
	key: "x" | "y",
): { min: number; span: number } => {
	requirePositiveLength(points.length, `latentRange.${key}`);

	const values = points.map((point) => point[key]);
	const min = Math.min(...values);
	const max = Math.max(...values);
	const span = max - min;

	if (!Number.isFinite(min) || !Number.isFinite(span)) {
		requirePositive(Number.NaN, `latentRange.${key}.span`);
	}

	return { min, span: requirePositive(span, `latentRange.${key}.span`) };
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
	const xSpan = requirePositive(xRange.span, "latentPointScreen.xSpan");
	const ySpan = requirePositive(yRange.span, "latentPointScreen.ySpan");

	return {
		x: pad + ((point.x - xRange.min) / xSpan) * (width - pad * 2),
		y: height - pad - ((point.y - yRange.min) / ySpan) * (height - pad * 2),
	};
};
