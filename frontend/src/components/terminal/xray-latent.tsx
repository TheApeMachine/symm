import { createRef } from "react";
import { appStore } from "#/collections/app";
import type { ResonanceFrame } from "#/collections/types";
import { terminalStore } from "#/collections/terminal";
import {
	clearCanvas,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	categoryColor,
	drawXrayWaiting,
	latentPointScreen,
	latentRange,
} from "#/components/terminal/xray-draw";
import {
	type LatentPoint,
	latentPointsFromFrames,
} from "#/components/terminal/xray-view";

const latentCanvasRef = createRef<HTMLCanvasElement>();
let latentPoints: LatentPoint[] = [];

/*
paintXrayLatent draws the current DRAW batch of resonance latent carriers into
latentCanvasRef. Only this batch is used — nothing is retained in JS.
*/
export const paintXrayLatent = (value: unknown, focusSymbol: string) => {
	const canvas = latentCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ResonanceFrame[];
	const points = latentPointsFromFrames(frames);
	latentPoints = points;
	const width = canvas.clientWidth;
	const height = canvas.clientHeight;

	if (points.length === 0) {
		drawXrayWaiting(context, width, height, "waiting for latent carriers");
		return;
	}

	clearCanvas(context, width, height);

	const pad = 28;
	let xRange: ReturnType<typeof latentRange>;
	let yRange: ReturnType<typeof latentRange>;

	try {
		xRange = latentRange(points, "x");
		yRange = latentRange(points, "y");
	} catch {
		drawXrayWaiting(context, width, height, "waiting for latent span");
		return;
	}

	context.strokeStyle = TERMINAL_COLORS.line;
	context.lineWidth = 1;

	for (let index = 0; index <= 4; index += 1) {
		const x = pad + index * ((width - pad * 2) / 4);
		const y = pad + index * ((height - pad * 2) / 4);

		context.beginPath();
		context.moveTo(x, pad);
		context.lineTo(x, height - pad);
		context.stroke();
		context.beginPath();
		context.moveTo(pad, y);
		context.lineTo(width - pad, y);
		context.stroke();
	}

	for (const point of points) {
		const focus = point.symbol === focusSymbol;
		const x =
			pad + ((point.x - xRange.min) / xRange.span) * (width - pad * 2);
		const y =
			height -
			pad -
			((point.y - yRange.min) / yRange.span) * (height - pad * 2);
		const color = categoryColor(point.category, focus);

		context.fillStyle = color;
		context.globalAlpha = focus ? 1 : 0.72;
		context.shadowBlur = focus ? 12 : 4;
		context.shadowColor = color;
		context.beginPath();
		context.arc(x, y, focus ? 5 : 3.5, 0, Math.PI * 2);
		context.fill();
		context.shadowBlur = 0;
		context.globalAlpha = 1;

		if (focus) {
			context.strokeStyle = TERMINAL_COLORS.amber;
			context.lineWidth = 1.5;
			context.beginPath();
			context.arc(x, y, 9, 0, Math.PI * 2);
			context.stroke();
			context.fillStyle = TERMINAL_COLORS.foreground;
			context.font = "9px JetBrains Mono, monospace";
			context.fillText(
				point.symbol.split("/")[0] ?? point.symbol,
				x + 11,
				y + 4,
			);
		}
	}
};

/*
XrayLatentPanel is the static latent scatter shell. DRAW paints via
paintXrayLatent; clicks update focus from the last painted points.
*/
export const XrayLatentPanel = () => (
	<canvas
		ref={latentCanvasRef}
		onClick={(event) => {
			const canvas = latentCanvasRef.current;
			const points = latentPoints;

			if (canvas === null || points.length === 0) {
				return;
			}

			const rect = canvas.getBoundingClientRect();
			const clickX = event.clientX - rect.left;
			const clickY = event.clientY - rect.top;
			let nearest: LatentPoint | null = null;
			let nearestDistance = Number.POSITIVE_INFINITY;

			for (const point of points) {
				const screen = latentPointScreen(
					point,
					points,
					canvas.clientWidth,
					canvas.clientHeight,
				);
				const distance = Math.hypot(screen.x - clickX, screen.y - clickY);

				if (distance < nearestDistance && distance <= 14) {
					nearest = point;
					nearestDistance = distance;
				}
			}

			if (nearest === null) {
				return;
			}

			appStore.actions.updateFocusSymbol(nearest.symbol);
			terminalStore.actions.selectFocusSymbol(nearest.symbol);
		}}
		className="absolute inset-0 block size-full cursor-pointer"
	/>
);
