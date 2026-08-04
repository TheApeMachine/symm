import {
	clearCanvas,
	drawGrid,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type { LatentPoint } from "#/components/terminal/xray-view";
import { requirePositiveLength } from "#/lib/domain";

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
latentAxis returns the projection from one latent coordinate onto a unit axis.

A single carrier — or several that settled on the same coordinate — has no
extent to normalise against. That is a real reading of the embedding rather than
missing data, so the axis collapses to its centre instead of refusing to draw.
*/
export const latentAxis = (
	points: LatentPoint[],
	key: "x" | "y",
): ((value: number) => number) => {
	requirePositiveLength(points.length, `latentAxis.${key}`);

	const values = points.map((point) => point[key]);
	const min = Math.min(...values);
	const span = Math.max(...values) - min;

	if (!Number.isFinite(min) || !Number.isFinite(span) || span <= 0) {
		return () => 0.5;
	}

	return (value: number) => (value - min) / span;
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
	const projectX = latentAxis(points, "x");
	const projectY = latentAxis(points, "y");

	return {
		x: pad + projectX(point.x) * (width - pad * 2),
		y: height - pad - projectY(point.y) * (height - pad * 2),
	};
};
