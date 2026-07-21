import { createRef } from "react";
import {
	Circular,
	type CircularBuffer,
	latestOf,
} from "#/collections/circular";
import type { ResonanceFrame } from "#/collections/types";
import { drawPredictiveCodingChart } from "#/components/charts/prediction/canvas";
import { resizeCanvas } from "#/components/terminal/canvas";

const predictionCanvasRef = createRef<HTMLCanvasElement>();
let predictionHistory = Circular<ResonanceFrame>(0);

/*
updatePredictionHistory applies websocket delta semantics to one bounded chart
history. A repeated symbol/timestamp updates the current observation; a new
timestamp appends. The circular capacity is derived by the painter from the
number of horizontal pixels available, so retained samples remain drawable.
*/
export const updatePredictionHistory = (
	history: CircularBuffer<ResonanceFrame>,
	frames: ResonanceFrame[],
): void => {
	for (const frame of frames) {
		const latest = latestOf(history);

		if (latest?.symbol === frame.symbol && latest.at === frame.at) {
			history.replaceTail(frame);
			continue;
		}

		history.push(frame);
	}
};

/*
replacePredictionHistory rebuilds the bounded canvas series from the central
retained projection. Rebuilding prevents an older retained prefix from being
re-appended whenever a new websocket observation arrives.
*/
const replacePredictionHistory = (
	capacity: number,
	frames: ResonanceFrame[],
): void => {
	predictionHistory = Circular<ResonanceFrame>(capacity);
	updatePredictionHistory(predictionHistory, frames.slice(-capacity));
};

/*
paintTerminalPredictionChart replaces its canvas series from the centrally
retained focused resonance history, so websocket deltas append exactly once.
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
	const capacity = Math.max(2, Math.floor(canvas.clientWidth));
	replacePredictionHistory(capacity, focused);

	drawPredictiveCodingChart(
		context,
		canvas.clientWidth,
		canvas.clientHeight,
		predictionHistory.values(),
	);
};

/*
TerminalPredictionChart is the static canvas shell. DRAW paints via
paintTerminalPredictionChart.
*/
export const TerminalPredictionChart = () => (
	<canvas ref={predictionCanvasRef} className="block size-full" />
);
