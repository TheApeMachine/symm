import type { PredictionSample } from "#/components/charts/prediction/series";
import { drawPolyline, TERMINAL_COLORS } from "#/components/terminal/canvas";

export const PREDICTION_LAYOUT = {
	top: 52,
	bottom: 26,
	labelWidth: 104,
	detailWidth: 158,
	gap: 4,
	padding: 9,
} as const;

/*
TraceScale maps a numeric series into one lane using bounds derived from the
observed values rather than fixed market thresholds.
*/
export type TraceScale = {
	minimum: number;
	maximum: number;
};

/*
finiteValues removes explicit gaps only while deriving a display extent.
*/
const finiteValues = (...series: PredictionSample[][]): number[] =>
	series.flat().filter((value): value is number => value !== null);

/*
positiveScale anchors reconstruction error and latent magnitude at their
meaningful zero while deriving the upper extent from the retained history.
*/
export const positiveScale = (values: PredictionSample[]): TraceScale => ({
	minimum: 0,
	maximum: Math.max(...finiteValues(values), Number.EPSILON),
});

/*
signedScale derives a zero-centered range for signed return forecasts and their
uncertainty band.
*/
export const signedScale = (...series: PredictionSample[][]): TraceScale => {
	const extent = Math.max(
		...finiteValues(...series).map((value) => Math.abs(value)),
		Number.EPSILON,
	);

	return { minimum: -extent, maximum: extent };
};

/*
traceSegments converts a gapped time series into drawable point runs. Missing
epochs therefore remain visible instead of being bridged by a line.
*/
export const traceSegments = (
	values: PredictionSample[],
	left: number,
	right: number,
	top: number,
	bottom: number,
	scale: TraceScale,
): Array<Array<{ x: number; y: number }>> => {
	const denominator = Math.max(values.length - 1, 1);
	const span = scale.maximum - scale.minimum;
	const segments: Array<Array<{ x: number; y: number }>> = [];
	let segment: Array<{ x: number; y: number }> = [];

	for (const [index, value] of values.entries()) {
		if (value === null) {
			if (segment.length > 0) {
				segments.push(segment);
				segment = [];
			}

			continue;
		}

		segment.push({
			x: left + (index / denominator) * (right - left),
			y: bottom - ((value - scale.minimum) / span) * (bottom - top),
		});
	}

	if (segment.length > 0) {
		segments.push(segment);
	}

	return segments;
};

/*
drawTrace paints every contiguous portion of a history and preserves its gaps.
*/
export const drawTrace = (
	context: CanvasRenderingContext2D,
	values: PredictionSample[],
	left: number,
	right: number,
	top: number,
	bottom: number,
	scale: TraceScale,
	color: string,
	dashed = false,
): void => {
	for (const points of traceSegments(values, left, right, top, bottom, scale)) {
		drawPolyline(context, points, color, dashed);
	}
};

/*
drawLaneFrame separates chart responsibilities and gives each layer a compact
two-line identity that remains readable in the dashboard's shortest layout.
*/
export const drawLaneFrame = (
	context: CanvasRenderingContext2D,
	width: number,
	top: number,
	bottom: number,
	label: string,
	metric: string,
): void => {
	const center = (top + bottom) / 2;

	context.strokeStyle = TERMINAL_COLORS.line;
	context.lineWidth = 1;
	context.beginPath();
	context.moveTo(14, bottom);
	context.lineTo(width - 14, bottom);
	context.stroke();
	context.fillStyle = TERMINAL_COLORS.foreground;
	context.fillText(label, 18, center - 3);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.fillText(metric, 18, center + 11);
};
