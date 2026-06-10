import {
	EArrowHeadPosition,
	EColumnMode,
	EColumnYMode,
	EFillPaletteMode,
	EHorizontalAnchorPoint,
	EVerticalAnchorPoint,
	FastRectangleRenderableSeries,
	type IFillPaletteProvider,
	type IRenderableSeries,
	LineArrowAnnotation,
	NativeTextAnnotation,
	NumberRange,
	NumericAxis,
	parseColorToUIntArgb,
	type SciChartSubSurface,
	XyxyDataSeries,
} from "scichart";

import { appTheme } from "#/components/charts/shared/theme";

const TRACK_HEIGHT = 8;
const SURPRISE_BAND_COLORS = [
	appTheme.VividGreen,
	appTheme.VividOrange,
	appTheme.VividRed,
] as const;

class SurpriseBandPalette implements IFillPaletteProvider {
	public readonly fillPaletteMode = EFillPaletteMode.SOLID;

	private readonly colors: number[];

	constructor(colorStrings: readonly string[]) {
		this.colors = colorStrings.map((color) => parseColorToUIntArgb(color));
	}

	public onAttached(_parentSeries: IRenderableSeries): void {}

	public onDetached(): void {}

	public overrideFillArgb(
		_xValue: number,
		_yValue: number,
		index: number,
	): number | undefined {
		return this.colors[index % this.colors.length];
	}
}

const segmentEdges = (
	threshold: number,
): readonly [number, number, number, number] => [
	0,
	threshold,
	threshold * 2,
	threshold * 3,
];

const buildBandSeries = (
	wasmContext: SciChartSubSurface["webAssemblyContext2D"],
	threshold: number,
): FastRectangleRenderableSeries => {
	const [start, mid, upper, end] = segmentEdges(threshold);
	const segments: Array<[number, number]> = [
		[start, mid],
		[mid, upper],
		[upper, end],
	];

	const xValues: number[] = [];
	const yValues: number[] = [];
	const x1Values: number[] = [];
	const y1Values: number[] = [];

	for (const [segmentStart, segmentEnd] of segments) {
		if (segmentEnd <= segmentStart) {
			continue;
		}

		xValues.push(segmentStart);
		yValues.push(0);
		x1Values.push(segmentEnd);
		y1Values.push(TRACK_HEIGHT);
	}

	return new FastRectangleRenderableSeries(wasmContext, {
		dataSeries: new XyxyDataSeries(wasmContext, {
			xValues,
			yValues,
			x1Values,
			y1Values,
			containsNaN: false,
			dataIsSortedInX: true,
		}),
		columnXMode: EColumnMode.StartEnd,
		columnYMode: EColumnYMode.TopBottom,
		strokeThickness: 0,
		paletteProvider: new SurpriseBandPalette(SURPRISE_BAND_COLORS),
	});
};

export type SurpriseSubChartControls = {
	update: (surprise: number, scaleMax: number, threshold: number) => void;
};

export const createSurpriseSubChart = (
	subChart: SciChartSubSurface,
): SurpriseSubChartControls => {
	const wasmContext = subChart.webAssemblyContext2D;

	const xAxis = new NumericAxis(wasmContext, {
		isVisible: false,
		growBy: new NumberRange(0, 0),
		visibleRange: new NumberRange(0, 6),
		useNativeText: true,
	});

	const yAxis = new NumericAxis(wasmContext, {
		isVisible: false,
		growBy: new NumberRange(2, 2),
	});

	subChart.xAxes.add(xAxis);
	subChart.yAxes.add(yAxis);

	let bandSeries = buildBandSeries(wasmContext, 2);
	subChart.renderableSeries.add(bandSeries);

	const pointer = new LineArrowAnnotation({
		y1: TRACK_HEIGHT,
		y2: TRACK_HEIGHT + 1,
		x1: 0,
		x2: 0,
		isArrowHeadScalable: true,
		arrowStyle: {
			headLength: 6,
			headWidth: 5,
			headDepth: 1,
			fill: appTheme.ForegroundColor,
			strokeThickness: 0,
		},
		stroke: appTheme.ForegroundColor,
		strokeThickness: 1.5,
		arrowHeadPosition: EArrowHeadPosition.Start,
	});

	const label = new NativeTextAnnotation({
		y1: TRACK_HEIGHT + 2,
		x1: 0,
		text: "0.00",
		fontSize: 9,
		textColor: appTheme.ForegroundColor,
		horizontalAnchorPoint: EHorizontalAnchorPoint.Center,
		verticalAnchorPoint: EVerticalAnchorPoint.Top,
	});

	subChart.annotations.add(pointer, label);

	let currentScaleMax = 6;
	let currentThreshold = 2;

	const replaceBandSeries = (threshold: number): void => {
		subChart.renderableSeries.remove(bandSeries);
		bandSeries = buildBandSeries(wasmContext, threshold);
		subChart.renderableSeries.add(bandSeries);
	};

	return {
		update(surprise: number, scaleMax: number, threshold: number) {
			if (subChart.isDeleted) {
				return;
			}

			const nextThreshold = Math.max(threshold, 0.1);
			const nextScaleMax = Math.max(scaleMax, nextThreshold * 3, 1);

			if (
				nextScaleMax !== currentScaleMax ||
				nextThreshold !== currentThreshold
			) {
				currentScaleMax = nextScaleMax;
				currentThreshold = nextThreshold;
				xAxis.visibleRange = new NumberRange(0, nextScaleMax);
				replaceBandSeries(nextThreshold);
			}

			const marker = Math.max(0, Math.min(surprise, nextScaleMax));

			pointer.x1 = marker;
			pointer.x2 = marker;
			label.x1 = marker;
			label.text = Math.max(0, surprise).toFixed(2);
			subChart.invalidateElement();
		},
	};
};
