import { createRef } from "react";
import type { ManifoldFrame } from "#/collections/types";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { frameMatrix } from "#/components/terminal/charts-frame";

const manifoldCanvasRef = createRef<HTMLCanvasElement>();

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
paintTerminalManifoldChart draws the current DRAW batch of manifold rho into
manifoldCanvasRef. Only this batch is used — nothing is retained in JS.
*/
export const paintTerminalManifoldChart = (
	value: unknown,
	focusSymbol: string,
) => {
	const canvas = manifoldCanvasRef.current;

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
	) as ManifoldFrame[];
	const frame =
		frames.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ??
		frames.at(-1) ??
		null;
	const matrix = frameMatrix(frame);

	if (matrix.length === 0) {
		drawWaiting(context, width, height, "waiting for manifold rho");
		return;
	}

	drawMatrix(context, width, height, matrix);
};

/*
TerminalManifoldChart is the static canvas shell. DRAW paints via
paintTerminalManifoldChart.
*/
export const TerminalManifoldChart = () => (
	<canvas ref={manifoldCanvasRef} className="block size-full" />
);
