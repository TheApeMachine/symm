import { createRef } from "react";
import type { ResonanceFrame } from "#/collections/types";
import {
	clearCanvas,
	drawGrid,
	drawPolyline,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { requireNonZero, requireSampleSize } from "#/lib/domain";

const predictionCanvasRef = createRef<HTMLCanvasElement>();

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
drawTerminalPredictionSparkline paints actual versus predicted sensory state
from a resonance frame batch already filtered for the focus symbol.
*/
const drawTerminalPredictionSparkline = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	frames: ResonanceFrame[],
): void => {
	if (frames.length < 2) {
		drawWaiting(context, width, height, "waiting for resonance history");
		return;
	}

	clearCanvas(context, width, height);
	drawGrid(context, width, height, 18);

	const values = frames.flatMap((frame) => {
		const layer = frame.layers?.[0];

		if (layer === undefined) {
			return [];
		}

		return [...layer.state, ...layer.prediction].filter(Number.isFinite);
	});

	if (values.length === 0) {
		return;
	}

	let min = Math.min(...values);
	let max = Math.max(...values);
	const span = max > min ? max - min : 1;
	const margin = span * 0.08;
	min -= margin;
	max += margin;
	const paddedSpan = max > min ? max - min : 1;
	const paddingX = 18;
	const plotWidth = Math.max(1, width - paddingX * 2);
	const plotHeight = Math.max(1, height - 46);
	requireSampleSize(frames.length, 2, "resonance sparkline");
	const denominator = requireNonZero(
		frames.length - 1,
		"resonance sparkline denominator",
	);
	const xFor = (index: number) => paddingX + (index / denominator) * plotWidth;
	const yFor = (value: number) =>
		height - 26 - ((value - min) / paddedSpan) * plotHeight;
	const actualPoints = frames.flatMap((frame, index) => {
		const state = frame.layers?.[0]?.state ?? [];
		const finite = state.filter(Number.isFinite);

		if (finite.length === 0) {
			return [];
		}

		const actual =
			finite.reduce((sum, entry) => sum + entry, 0) / finite.length;

		return [{ x: xFor(index), y: yFor(actual) }];
	});
	const predictionPoints = frames.flatMap((frame, index) => {
		const prediction = frame.layers?.[0]?.prediction ?? [];
		const finite = prediction.filter(Number.isFinite);

		if (finite.length === 0) {
			return [];
		}

		const value = finite.reduce((sum, entry) => sum + entry, 0) / finite.length;

		return [{ x: xFor(index), y: yFor(value) }];
	});

	if (actualPoints.length === 0 || predictionPoints.length === 0) {
		drawWaiting(context, width, height, "waiting for resonance layers");
		return;
	}

	context.fillStyle = "rgba(232, 163, 61, 0.18)";
	context.beginPath();
	for (const [index, point] of actualPoints.entries()) {
		if (index === 0) {
			context.moveTo(point.x, point.y);
		} else {
			context.lineTo(point.x, point.y);
		}
	}
	for (let index = predictionPoints.length - 1; index >= 0; index -= 1) {
		const point = predictionPoints[index];
		context.lineTo(point.x, point.y);
	}
	context.closePath();
	context.fill();

	drawPolyline(context, actualPoints, TERMINAL_COLORS.foreground);
	drawPolyline(context, predictionPoints, TERMINAL_COLORS.cyan, true);

	const latest = frames.at(-1);
	const latestActual = actualPoints.at(-1);
	const surprise = latest?.surprise;

	if (
		latestActual !== undefined &&
		typeof surprise === "number" &&
		Number.isFinite(surprise)
	) {
		context.fillStyle = TERMINAL_COLORS.amber;
		context.beginPath();
		context.arc(latestActual.x, latestActual.y, 2.6, 0, Math.PI * 2);
		context.fill();
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "10px JetBrains Mono, monospace";
		context.fillText(`ε ${surprise.toFixed(4)}`, 18, height - 8);
	}
};

/*
paintTerminalPredictionChart draws the current DRAW batch of resonance frames
into predictionCanvasRef. Only this batch is used — nothing is retained in JS.
*/
export const paintTerminalPredictionChart = (
	value: unknown,
	focusSymbol: string,
) => {
	const canvas = predictionCanvasRef.current;

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
	const focused = frames.filter(
		(frame) => focusSymbol === "" || frame.symbol === focusSymbol,
	);

	drawTerminalPredictionSparkline(
		context,
		canvas.clientWidth,
		canvas.clientHeight,
		focused,
	);
};

/*
TerminalPredictionChart is the static canvas shell. DRAW paints via
paintTerminalPredictionChart.
*/
export const TerminalPredictionChart = () => (
	<canvas ref={predictionCanvasRef} className="block size-full" />
);
