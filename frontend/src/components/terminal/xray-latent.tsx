import { createRef } from "react";
import { createStore } from "@tanstack/store";
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
	latentAxis,
	latentPointScreen,
} from "#/components/terminal/xray-draw";
import {
	type LatentPoint,
	latentPointsFromFrames,
} from "#/components/terminal/xray-view";

const latentCanvasRef = createRef<HTMLCanvasElement>();
const latentPointsStore = createStore<LatentPoint[]>([]);
let latentFingerprint = "";
let latentGeometry = "";

const retainedLatentPoints = (): LatentPoint[] =>
	[...latentPointsStore.state].sort((left, right) =>
		left.symbol.localeCompare(right.symbol),
	);

const latentKey = (point: LatentPoint): string =>
	`${point.symbol}|${point.x.toFixed(6)}|${point.y.toFixed(6)}|${point.category}`;

const latentSignature = (points: LatentPoint[], focusSymbol: string): string =>
	`${focusSymbol}::${points.map(latentKey).join(";")}`;

/*
paintXrayLatent draws the resonance store's complete bounded carrier snapshot.
It retains only projected canvas geometry, not another copy of backend rows.
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
	latentPointsStore.setState(() => latentPointsFromFrames(frames));
	const pointsToDraw = retainedLatentPoints();
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

	/*
		Every carrier is named. The embedding routinely holds a handful of symbols
		at most, and an unlabelled dot says nothing about which one settled where.
	*/
	for (const point of pointsToDraw) {
		const focus = point.symbol === focusSymbol;
		const x = pad + projectX(point.x) * (width - pad * 2);
		const y = height - pad - projectY(point.y) * (height - pad * 2);
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
		}

		context.fillStyle = focus
			? TERMINAL_COLORS.foreground
			: TERMINAL_COLORS.muted;
		context.font = "9px JetBrains Mono, monospace";
		context.fillText(
			point.symbol.split("/")[0] ?? point.symbol,
			x + (focus ? 11 : 7),
			y + 4,
		);
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
			const points = retainedLatentPoints();

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
