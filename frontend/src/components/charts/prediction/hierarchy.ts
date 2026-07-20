import type { HierarchyTrace } from "#/components/charts/prediction/series";
import {
	drawLaneFrame,
	drawTrace,
	PREDICTION_LAYOUT,
	positiveScale,
} from "#/components/charts/prediction/trace";
import { TERMINAL_COLORS } from "#/components/terminal/canvas";

/*
drawProfile compares every current state component with its top-down prediction;
the macro layer intentionally renders state alone because it has no parent link.
*/
const drawProfile = (
	context: CanvasRenderingContext2D,
	trace: HierarchyTrace,
	left: number,
	right: number,
	top: number,
	bottom: number,
): void => {
	const prediction = trace.prediction ?? [];
	const extent = Math.max(
		...trace.state.map(Math.abs),
		...prediction.map(Math.abs),
		Number.EPSILON,
	);
	const scale = { minimum: -extent, maximum: extent };

	drawTrace(
		context,
		trace.state,
		left,
		right,
		top,
		bottom,
		scale,
		TERMINAL_COLORS.foreground,
	);

	if (prediction.length > 0) {
		drawTrace(
			context,
			prediction,
			left,
			right,
			top,
			bottom,
			scale,
			TERMINAL_COLORS.cyan,
			true,
		);
	}
};

/*
drawHierarchyLane paints reconstruction error or top-context activity through
time and the latest complete component profile beside it.
*/
export const drawHierarchyLane = (
	context: CanvasRenderingContext2D,
	width: number,
	top: number,
	bottom: number,
	trace: HierarchyTrace,
): void => {
	const detailLeft = width - PREDICTION_LAYOUT.detailWidth;
	const plotRight = detailLeft - 16;
	const scale = positiveScale(trace.values);
	const latest = trace.values.at(-1);
	const label = trace.kind === "reconstruction" ? "top-down ε₂" : "state ‖z‖₂";
	const color =
		trace.kind === "reconstruction"
			? TERMINAL_COLORS.amber
			: TERMINAL_COLORS.green;
	const metric = `${label} ${latest == null ? "—" : latest.toFixed(4)}`;

	drawLaneFrame(context, width, top, bottom, trace.label, metric);
	drawTrace(
		context,
		trace.values,
		PREDICTION_LAYOUT.labelWidth,
		plotRight,
		top + PREDICTION_LAYOUT.padding,
		bottom - PREDICTION_LAYOUT.padding,
		scale,
		color,
	);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.fillText(
		`max ${scale.maximum.toFixed(3)}`,
		PREDICTION_LAYOUT.labelWidth,
		top + 10,
	);
	context.fillText(
		trace.prediction === null ? "NOW z" : "NOW z / ẑ",
		detailLeft,
		top + 10,
	);
	drawProfile(context, trace, detailLeft, width - 18, top + 14, bottom - 5);
};
