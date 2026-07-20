import type { ResonanceFrame } from "#/collections/types";
import { drawReturnLane } from "#/components/charts/prediction/head";
import { drawHierarchyLane } from "#/components/charts/prediction/hierarchy";
import { predictiveCodingSeries } from "#/components/charts/prediction/series";
import { PREDICTION_LAYOUT } from "#/components/charts/prediction/trace";
import { clearCanvas, TERMINAL_COLORS } from "#/components/terminal/canvas";

const FONT = "10px JetBrains Mono, monospace";

/*
drawPredictiveCodingChart renders the complete online hierarchy and its distinct
return task head from the bounded symbol-local resonance history.
*/
export const drawPredictiveCodingChart = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	frames: ResonanceFrame[],
): void => {
	clearCanvas(context, width, height);
	const series = predictiveCodingSeries(frames);

	if (series.layers.length === 0) {
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = FONT;
		context.fillText("waiting for resonance layers", 18, PREDICTION_LAYOUT.top);

		return;
	}

	const laneCount = series.layers.length + 1;
	const availableHeight =
		height - PREDICTION_LAYOUT.top - PREDICTION_LAYOUT.bottom;
	const laneHeight =
		(availableHeight - PREDICTION_LAYOUT.gap * (laneCount - 1)) / laneCount;
	context.font = FONT;

	for (const [index, layer] of series.layers.entries()) {
		const top =
			PREDICTION_LAYOUT.top + index * (laneHeight + PREDICTION_LAYOUT.gap);
		drawHierarchyLane(context, width, top, top + laneHeight, layer);
	}

	const returnTop =
		PREDICTION_LAYOUT.top +
		series.layers.length * (laneHeight + PREDICTION_LAYOUT.gap);
	drawReturnLane(
		context,
		width,
		returnTop,
		returnTop + laneHeight,
		series.returnHead,
	);
};
