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
let predictionHistoryCapacity = 0;
let predictionHistoryFocus = "";

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
resizePredictionHistory keeps the newest drawable samples when canvas width or
focus changes. A focus change deliberately starts a new series so symbols are
never joined by a meaningless line segment.
*/
const resizePredictionHistory = (
	focusSymbol: string,
	capacity: number,
): void => {
	if (
		focusSymbol === predictionHistoryFocus &&
		capacity === predictionHistoryCapacity
	) {
		return;
	}

	const retained =
		focusSymbol === predictionHistoryFocus
			? predictionHistory.values().slice(-capacity)
			: [];
	predictionHistory = Circular<ResonanceFrame>(capacity);
	updatePredictionHistory(predictionHistory, retained);
	predictionHistoryCapacity = capacity;
	predictionHistoryFocus = focusSymbol;
};

/*
paintTerminalPredictionChart appends resonance deltas for the focused symbol
and repaints its bounded time series. Unrelated symbol deltas leave the current
series intact instead of replacing it with an empty DRAW batch.
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
	resizePredictionHistory(focusSymbol, capacity);
	updatePredictionHistory(predictionHistory, focused);

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
