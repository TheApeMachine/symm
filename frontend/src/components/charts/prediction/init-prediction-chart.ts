import {
	DiscontinuousDateAxis,
	EAutoRange,
	EAxisAlignment,
	ENumericFormat,
	FastLineRenderableSeries,
	NumberRange,
	NumericAxis,
	SciChartSurface,
	XyDataSeries,
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
import { ensureSciChartWasm } from "#/lib/utils";

const SERIES_CAPACITY = 600;

export const initPredictionChart = async (rootElement: HTMLDivElement) => {
	await ensureSciChartWasm();

	const { wasmContext, sciChartSurface } = await SciChartSurface.create(
		rootElement,
		{ freezeWhenOutOfView: true },
	);

	const xAxis = new DiscontinuousDateAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Bottom,
		labelFormat: ENumericFormat.Date_HHMMSS,
		growBy: new NumberRange(0.02, 0),
		drawMajorBands: false,
		drawMinorGridLines: false,
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

	let horizonSec = 60;

	const visibleXRange = () =>
		predictionVisibleXRange(
			horizonSec,
			seriesLatestX(forecastSeries),
			seriesEarliestX(actualSeries),
			seriesEarliestX(errorSeries),
			Math.floor(Date.now() / 1000),
		);

	const syncXRange = () => {
		const { minX, maxX } = visibleXRange();

		xAxis.visibleRange = new NumberRange(minX, maxX);
	};

	syncXRange();

	const addData = (frame: Record<string, unknown>) => {
		const kind = frame.kind;

		if (kind !== "actual" && kind !== "prediction" && kind !== "error") {
			return;
		}

		const xValue = frame.x;
		const pointValue = frame.value;

		if (
			typeof xValue !== "number" ||
			!Number.isFinite(xValue) ||
			typeof pointValue !== "number" ||
			!Number.isFinite(pointValue)
		) {
			return;
		}

		const horizon = frame.horizon;

		if (
			typeof horizon === "number" &&
			Number.isFinite(horizon) &&
			horizon > 0
		) {
			horizonSec = horizon;
		}

		const { minX } = visibleXRange();

		if (kind === "prediction") {
			upsertSortedPoint(forecastSeries, xValue, pointValue);
		}

		if (kind === "actual") {
			upsertSortedPoint(actualSeries, xValue, pointValue);
		}

		if (kind === "error") {
			upsertSortedPoint(errorSeries, xValue, pointValue);
		}

		pruneSeriesBefore(forecastSeries, minX);
		pruneSeriesBefore(actualSeries, minX);
		pruneSeriesBefore(errorSeries, minX);

		syncXRange();
		sciChartSurface.invalidateElement();
	};

	return {
		sciChartSurface,
		addData,
	};
};
