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
let predictionFocus = "";

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

const resetPredictionHistory = (capacity: number): void => {
	predictionHistory = Circular<ResonanceFrame>(capacity);
};

/*
hasPredictionLayers confirms that at least one incoming resonance frame still
carries settled hierarchy layers. Sparse meta-only rows must not blank the
retained chart history while the backend catches up.
*/
const hasPredictionLayers = (frames: ResonanceFrame[]): boolean =>
	frames.some(
		(frame) => Array.isArray(frame.layers) && frame.layers.length > 0,
	);

export const shouldReplacePredictionHistory = (frames: ResonanceFrame[]): boolean =>
	hasPredictionLayers(frames);

/*
paintTerminalPredictionChart incrementally merges focused resonance frames into
one stable local history buffer. Sparse websocket projections must update the
existing curve in place rather than rebuilding and flickering the entire panel.
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
	const focusChanged = predictionFocus !== focusSymbol;

	if (focusChanged || predictionHistory.capacity() !== capacity) {
		predictionFocus = focusSymbol;
		resetPredictionHistory(capacity);
	}

	if (shouldReplacePredictionHistory(focused)) {
		updatePredictionHistory(predictionHistory, focused.slice(-capacity));
	}

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
