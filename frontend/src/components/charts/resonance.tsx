import { createRef } from "react";
import type { ResonanceFrame } from "#/collections/types";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { terminalResonanceLayerMatrixFromFrame } from "#/components/terminal/charts-frame";

const resonanceCanvasRef = createRef<HTMLCanvasElement>();

const drawWaiting = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	message: string,
) => {
	clearCanvas(context, width, height);
	drawGrid(context, width, height);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "11px JetBrains Mono, monospace";
	context.fillText(message, 18, 52);
};

/*
paintTerminalResonanceChart draws the current DRAW batch of resonance layers into
resonanceCanvasRef. Only this batch is used — nothing is retained in JS.
*/
export const paintTerminalResonanceChart = (
	value: unknown,
	focusSymbol: string,
) => {
	const canvas = resonanceCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	
	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ResonanceFrame[];

	const frame =
		frames.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ??
		frames.at(-1) ??
		null;
	
		const matrix = terminalResonanceLayerMatrixFromFrame(frame);

	if (matrix.length === 0) {
		drawWaiting(context, width, height, "waiting for resonance layers");
		return;
	}

	drawMatrix(context, width, height, matrix);
};

/*
TerminalResonanceChart is the static canvas shell. DRAW paints via
paintTerminalResonanceChart.
*/
export const TerminalResonanceChart = () => (
	<canvas ref={resonanceCanvasRef} className="block size-full" />
);
