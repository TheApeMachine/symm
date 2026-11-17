import { createRef } from "react";
import { atoms } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	clearCanvas,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	categoryColor,
	drawXrayWaiting,
	latentAxis,
	latentPointScreen,
} from "#/components/terminal/xray-draw";
import {
	type LatentPoint,
	latentPointsFromFrames,
} from "#/components/terminal/xray-view";

const latentCanvasRef = createRef<HTMLCanvasElement>();
let lastPaintedPoints: LatentPoint[] = [];
let latentFingerprint = "";
let latentGeometry = "";

const latentKey = (point: LatentPoint): string =>
	`${point.symbol}|${point.x.toFixed(6)}|${point.y.toFixed(6)}|${point.category}`;

const latentSignature = (points: LatentPoint[], focusSymbol: string): string =>
	`${focusSymbol}::${points.map(latentKey).join(";")}`;

export const paintXrayLatent = (value: unknown, focusSymbol: string) => {
	const canvas = latentCanvasRef.current;
	if (canvas === null) return;

	const context = resizeCanvas(canvas);
	if (context === null) return;

	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Array<Record<string, unknown>>;
	const pointsToDraw = latentPointsFromFrames(frames).sort((left, right) =>
		left.symbol.localeCompare(right.symbol),
	);
	lastPaintedPoints = pointsToDraw;

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	const geometry = `${width}x${height}`;
	const fingerprint = latentSignature(pointsToDraw, focusSymbol);

	if (pointsToDraw.length === 0) {
		latentFingerprint = "";
		latentGeometry = geometry;
		drawXrayWaiting(context, width, height, "waiting for latent carriers");
		return;
	}

	if (fingerprint === latentFingerprint && geometry === latentGeometry) {
		return;
	}

	latentFingerprint = fingerprint;
	latentGeometry = geometry;

	clearCanvas(context, width, height);

	const pad = 28;
	const projectX = latentAxis(pointsToDraw, "x");
	const projectY = latentAxis(pointsToDraw, "y");

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

	for (let index = 0; index < pointsToDraw.length; index += 1) {
		const point = pointsToDraw[index];
		if (point === undefined) continue;

		const focus = point.symbol === focusSymbol;
		const x = pad + projectX(point.x) * (width - pad * 2);
		const y = height - pad - projectY(point.y) * (height - pad * 2);
		const color = categoryColor(point.category, focus, index);

		context.fillStyle = color;
		context.globalAlpha = focus ? 1 : 0.65;
		context.shadowBlur = focus ? 14 : 0;
		context.shadowColor = color;
		context.beginPath();
		context.arc(x, y, focus ? 6 : 3.5, 0, Math.PI * 2);
		context.fill();
		context.shadowBlur = 0;
		context.globalAlpha = 1;

		if (focus) {
			context.strokeStyle = TERMINAL_COLORS.amber;
			context.lineWidth = 1.4;
			context.beginPath();
			context.arc(x, y, 11, 0, Math.PI * 2);
			context.stroke();

			context.fillStyle = TERMINAL_COLORS.foreground;
			context.font = "10px JetBrains Mono, monospace";
			context.fillText(
				point.symbol.split("/")[0] ?? point.symbol,
				x + 15,
				y + 3.5,
			);
		}
	}
};

export const XrayLatentPanel = () => (
	<canvas
		ref={latentCanvasRef}
		onClick={(event) => {
			const canvas = latentCanvasRef.current;
			const points = lastPaintedPoints;

			if (canvas === null || points.length === 0) return;

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

			if (nearest === null) return;

			atoms.focus.set(nearest.symbol);
			terminalStore.actions.selectFocusSymbol(nearest.symbol);
		}}
		className="absolute inset-0 block size-full cursor-pointer"
	/>
);