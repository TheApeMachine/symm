import type { OhlcDataSeries } from "scichart";
import {
	EAnnotationVisibilityMode,
	EAxisLabelDrawMode,
	EMultiPointLabelAnchorMode,
	StopLossTakeProfitAnnotation,
} from "scichart-financial-tools";

import type { FinancialChartContext } from "#/components/charts/shared/financial-chart-utils";

export type StopLossOverlayInput = {
	avgEntry: number;
	stopPrice: number;
};

export const STOP_LOSS_ANNOTATION_ID = "symm-stop-loss";

const STOP_LOSS_COLOR = "#C52E60";
const TAKE_PROFIT_COLOR = "#67BDAF";

export const stopLossAnnotationPoints = (
	ohlc: OhlcDataSeries,
	position: StopLossOverlayInput,
): { x0: number; y0: number; x1: number; y1: number } | null => {
	if (position.avgEntry <= 0 || position.stopPrice <= 0) {
		return null;
	}

	const barCount = ohlc.count();

	if (barCount <= 0) {
		return null;
	}

	const nativeX = ohlc.getNativeXValues();

	return {
		x0: nativeX.get(0),
		y0: position.avgEntry,
		x1: nativeX.get(barCount - 1),
		y1: position.stopPrice,
	};
};

export const stopLossOverlayFromPosition = (
	position:
		| {
				avgEntry: number;
				stopPrice?: number;
		  }
		| undefined,
): StopLossOverlayInput | null => {
	if (position === undefined) {
		return null;
	}

	if (position.avgEntry <= 0 || position.stopPrice === undefined) {
		return null;
	}

	if (position.stopPrice <= 0) {
		return null;
	}

	return {
		avgEntry: position.avgEntry,
		stopPrice: position.stopPrice,
	};
};

const stopLossPointsEqual = (
	left: StopLossOverlayInput,
	right: StopLossOverlayInput,
	previousX0: number,
	previousX1: number,
	nextX0: number,
	nextX1: number,
): boolean =>
	left.avgEntry === right.avgEntry &&
	left.stopPrice === right.stopPrice &&
	previousX0 === nextX0 &&
	previousX1 === nextX1;

export const syncStopLossAnnotation = (
	chart: FinancialChartContext,
	annotation: StopLossTakeProfitAnnotation | null,
	overlay: StopLossOverlayInput | null,
	previous: {
		overlay: StopLossOverlayInput;
		x0: number;
		x1: number;
	} | null,
): {
	annotation: StopLossTakeProfitAnnotation | null;
	previous: { overlay: StopLossOverlayInput; x0: number; x1: number } | null;
} => {
	const points = overlay ? stopLossAnnotationPoints(chart.ohlc, overlay) : null;

	if (points === null || overlay === null) {
		if (annotation !== null) {
			chart.sciChartSurface.annotations.remove(annotation, true);
		}

		return { annotation: null, previous: null };
	}

	if (
		annotation !== null &&
		previous !== null &&
		stopLossPointsEqual(
			previous.overlay,
			overlay,
			previous.x0,
			previous.x1,
			points.x0,
			points.x1,
		)
	) {
		return { annotation, previous };
	}

	const nextPoints = [
		{ x: points.x0, y: points.y0 },
		{ x: points.x1, y: points.y1 },
	];

	if (annotation === null) {
		const created = new StopLossTakeProfitAnnotation({
			id: STOP_LOSS_ANNOTATION_ID,
			isEditable: false,
			points: nextPoints,
			strokeThickness: 2,
			strokeDashArray: [6, 3],
			stopLossColor: STOP_LOSS_COLOR,
			takeProfitColor: TAKE_PROFIT_COLOR,
			fillOpacity: 0.18,
			axisSpanFillOpacity: 0.2,
			axisLabelVisibility: EAnnotationVisibilityMode.Always,
			axisLabelStroke: "#FFFFFF",
			labels: [
				{
					id: "stop-axis",
					anchorMode: EMultiPointLabelAnchorMode.Axis,
					axisLabelDrawMode: EAxisLabelDrawMode.Y,
					pointIndex: 1,
					text: "Stop",
				},
			],
		});

		chart.sciChartSurface.annotations.add(created);

		return {
			annotation: created,
			previous: { overlay, x0: points.x0, x1: points.x1 },
		};
	}

	annotation.points = nextPoints;
	chart.sciChartSurface.invalidateElement();

	return {
		annotation,
		previous: { overlay, x0: points.x0, x1: points.x1 },
	};
};
