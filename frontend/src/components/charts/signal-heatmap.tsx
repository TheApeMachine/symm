import { createRef } from "react";
import type { Measurement } from "#/collections/types";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { frameRows } from "#/providers/frame-history";

const signalHeatmapCanvasRef = createRef<HTMLCanvasElement>();
let signalHeatmapKind: "confidence" | "surprise" = "confidence";

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
paintTerminalSignalHeatmap draws retained measurement-category scores as an
oldest-first heatmap in signalHeatmapCanvasRef.
*/
export const paintTerminalSignalHeatmap = (
	value: unknown,
	_focusSymbol: string,
) => {
	const canvas = signalHeatmapCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	const measurements = frameRows<Measurement>(value);
	const matrix = measurements.flatMap((frame) => {
		const category = frame.categories?.at(0);
		const entry =
			signalHeatmapKind === "confidence"
				? category?.confidence
				: category?.surprisal;

		return typeof entry === "number" ? [[entry]] : [];
	});

	if (matrix.length === 0) {
		drawWaiting(context, width, height, "waiting for signal readings");
		return;
	}

	drawMatrix(context, width, height, matrix);
};

/*
TerminalSignalHeatmap is the static canvas shell. DRAW paints via
paintTerminalSignalHeatmap.
*/
export const TerminalSignalHeatmap = ({
	kind,
}: {
	kind: "confidence" | "surprise";
}) => {
	signalHeatmapKind = kind;

	return <canvas ref={signalHeatmapCanvasRef} className="block size-full" />;
};
