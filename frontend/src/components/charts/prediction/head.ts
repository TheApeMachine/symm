import type { ReturnHeadTrace } from "#/components/charts/prediction/series";
import {
	drawLaneFrame,
	drawTrace,
	PREDICTION_LAYOUT,
	signedScale,
	type TraceScale,
	traceSegments,
} from "#/components/charts/prediction/trace";
import { TERMINAL_COLORS } from "#/components/terminal/canvas";

/*
fillUncertainty paints each contiguous empirical residual interval around the
return head's strict-prior forecast without spanning missing observations.
*/
const fillUncertainty = (
	context: CanvasRenderingContext2D,
	head: ReturnHeadTrace,
	left: number,
	right: number,
	top: number,
	bottom: number,
	scale: TraceScale,
): void => {
	const upperSegments = traceSegments(
		head.upper,
		left,
		right,
		top,
		bottom,
		scale,
	);
	const lowerSegments = traceSegments(
		head.lower,
		left,
		right,
		top,
		bottom,
		scale,
	);
	context.fillStyle = "rgba(156, 192, 110, 0.12)";

	for (const [index, upper] of upperSegments.entries()) {
		const lower = lowerSegments[index] ?? [];

		if (upper.length < 2 || upper.length !== lower.length) {
			continue;
		}

		context.beginPath();
		context.moveTo(upper[0]?.x ?? left, upper[0]?.y ?? top);

		for (const point of upper.slice(1)) {
			context.lineTo(point.x, point.y);
		}

		for (const point of [...lower].reverse()) {
			context.lineTo(point.x, point.y);
		}

		context.closePath();
		context.fill();
	}
};

/*
drawReturnStats prints calibration evidence beside the forecast so readiness is
inspectable rather than represented by color alone.
*/
const drawReturnStats = (
	context: CanvasRenderingContext2D,
	head: ReturnHeadTrace,
	left: number,
	top: number,
): void => {
	context.fillStyle = TERMINAL_COLORS.muted;
	context.fillText(
		`μ ${head.latestExpected?.toExponential(2) ?? "—"} · σ ${head.latestUncertainty?.toExponential(2) ?? "—"}`,
		left,
		top + 14,
	);
	context.fillText(
		`conf ${head.confidence === null ? "—" : `${(head.confidence * 100).toFixed(0)}%`} · k ${head.horizon ?? "—"}`,
		left,
		top + 28,
	);
};

/*
drawReturnLane paints the distinct supervised task head, including uncertainty,
zero reference, readiness, and strict-prior calibration diagnostics.
*/
export const drawReturnLane = (
	context: CanvasRenderingContext2D,
	width: number,
	top: number,
	bottom: number,
	head: ReturnHeadTrace,
): void => {
	const detailLeft = width - PREDICTION_LAYOUT.detailWidth;
	const plotRight = detailLeft - 16;
	const scale = signedScale(head.lower, head.upper, head.expected);
	const zero = (top + bottom) / 2;
	const status = head.ready ? "FORECASTING" : "CALIBRATING";

	drawLaneFrame(
		context,
		width,
		top,
		bottom,
		"TASK · return",
		`strict-prior · ${status} · n ${head.samples ?? "—"}`,
	);
	context.strokeStyle = TERMINAL_COLORS.lineStrong;
	context.setLineDash([2, 3]);
	context.beginPath();
	context.moveTo(PREDICTION_LAYOUT.labelWidth, zero);
	context.lineTo(plotRight, zero);
	context.stroke();
	context.setLineDash([]);
	fillUncertainty(
		context,
		head,
		PREDICTION_LAYOUT.labelWidth,
		plotRight,
		top + PREDICTION_LAYOUT.padding,
		bottom - PREDICTION_LAYOUT.padding,
		scale,
	);
	drawTrace(
		context,
		head.expected,
		PREDICTION_LAYOUT.labelWidth,
		plotRight,
		top + PREDICTION_LAYOUT.padding,
		bottom - PREDICTION_LAYOUT.padding,
		scale,
		TERMINAL_COLORS.green,
	);
	drawReturnStats(context, head, detailLeft, top);
};
