import {
	EAutoRange,
	EAxisAlignment,
	ENumericFormat,
	FastLineRenderableSeries,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	SciChartSurface,
	XyDataSeries,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";
import {
	PREDICTION_VALUE_MAX,
	PREDICTION_VALUE_MIN,
	predictionVisibleXRange,
	pruneSeriesBefore,
	seriesEarliestX,
	seriesLatestX,
	upsertSortedPoint,
} from "#/components/charts/prediction/prediction-chart-series";
import type { PredictionPoint } from "#/components/charts/prediction/prediction-chart-wire";
import { ensureSciChartWasm } from "#/lib/utils";

export type TPredictionChartInitResult = {
	sciChartSurface: SciChartSurface;
	appendPoint: (point: PredictionPoint) => void;
};

const SERIES_CAPACITY = 600;

export const initPredictionChart = async (
	rootElement: HTMLDivElement,
): Promise<TPredictionChartInitResult> => {
	await ensureSciChartWasm();

	const { wasmContext, sciChartSurface } = await SciChartSurface.create(
		rootElement,
		{ freezeWhenOutOfView: true },
	);

	const xAxis = new NumericAxis(wasmContext, {
		labelFormat: ENumericFormat.Date_HHMMSS,
		growBy: new NumberRange(0.02, 0),
		labelStyle: {
			fontSize: 10,
		},
	});

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Left,
		autoRange: EAutoRange.Never,
		visibleRange: new NumberRange(PREDICTION_VALUE_MIN, PREDICTION_VALUE_MAX),
		growBy: new NumberRange(0.05, 0.05),
		labelStyle: {
			fontSize: 10,
		},
	});

	sciChartSurface.xAxes.add(xAxis);
	sciChartSurface.yAxes.add(yAxis);

	const forecastSeries = new XyDataSeries(wasmContext, {
		dataSeriesName: "Mean forecast (60s)",
		dataIsSortedInX: true,
		containsNaN: false,
		capacity: SERIES_CAPACITY,
	});

	const actualSeries = new XyDataSeries(wasmContext, {
		dataSeriesName: "Mean actual",
		dataIsSortedInX: true,
		containsNaN: false,
		capacity: SERIES_CAPACITY,
	});

	const errorSeries = new XyDataSeries(wasmContext, {
		dataSeriesName: "Mean error",
		dataIsSortedInX: true,
		containsNaN: false,
		capacity: SERIES_CAPACITY,
	});

	sciChartSurface.renderableSeries.add(
		new FastLineRenderableSeries(wasmContext, {
			dataSeries: forecastSeries,
			stroke: "#FBA55A",
			strokeThickness: 2,
			strokeDashArray: [6, 4],
		}),
		new FastLineRenderableSeries(wasmContext, {
			dataSeries: actualSeries,
			stroke: "#4EC385",
			strokeThickness: 2,
		}),
		new FastLineRenderableSeries(wasmContext, {
			dataSeries: errorSeries,
			stroke: "#E85D75",
			strokeThickness: 2,
		}),
	);

	sciChartSurface.chartModifiers.add(
		new ZoomExtentsModifier({ modifierGroup: "chart" }),
		new MouseWheelZoomModifier({ modifierGroup: "chart" }),
		new ZoomPanModifier({ modifierGroup: "chart" }),
	);

	let horizonSec = 60;

	const visibleXRange = (): { minX: number; maxX: number } =>
		predictionVisibleXRange(
			horizonSec,
			seriesLatestX(forecastSeries),
			seriesEarliestX(actualSeries),
			seriesEarliestX(errorSeries),
			Math.floor(Date.now() / 1000),
		);

	const syncXRange = (): void => {
		const { minX, maxX } = visibleXRange();

		xAxis.visibleRange = new NumberRange(minX, maxX);
	};

	syncXRange();

	const appendPoint = (point: PredictionPoint) => {
		if (!Number.isFinite(point.x) || !Number.isFinite(point.value)) {
			return;
		}

		if (point.horizon != null && point.horizon > 0) {
			horizonSec = point.horizon;
		}

		const { minX } = visibleXRange();

		if (point.kind === "prediction") {
			upsertSortedPoint(forecastSeries, point.x, point.value);
		}

		if (point.kind === "actual") {
			upsertSortedPoint(actualSeries, point.x, point.value);
		}

		if (point.kind === "error") {
			upsertSortedPoint(errorSeries, point.x, point.value);
		}

		pruneSeriesBefore(forecastSeries, minX);
		pruneSeriesBefore(actualSeries, minX);
		pruneSeriesBefore(errorSeries, minX);

		syncXRange();
		sciChartSurface.invalidateElement();
	};

	return {
		sciChartSurface,
		appendPoint,
	};
};
