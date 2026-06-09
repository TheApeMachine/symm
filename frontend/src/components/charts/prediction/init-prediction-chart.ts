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

import type { PredictionReading } from "#/components/charts/prediction/prediction-chart-wire";
import { ensureSciChartWasm } from "#/lib/utils";

type PredictionSeriesKind = PredictionReading["kind"];

const SERIES_STYLE: Record<
	PredictionSeriesKind,
	{
		name: string;
		stroke: string;
		strokeDashArray?: number[];
		strokeThickness: number;
	}
> = {
	actual: {
		name: "Actual forward movement",
		stroke: "#4EC385",
		strokeThickness: 2,
	},
	prediction: {
		name: "Forward forecast",
		stroke: "#FBA55A",
		strokeDashArray: [8, 5],
		strokeThickness: 2,
	},
	error: {
		name: "Catch-up forecast miss",
		stroke: "#E85D75",
		strokeThickness: 1,
	},
};

export type TPredictionChartInitResult = {
	sciChartSurface: SciChartSurface;
	appendReading: (reading: PredictionReading) => void;
};

const PREDICTION_FIFO_CAPACITY = 3600;

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
		visibleRange: new NumberRange(
			Math.floor(Date.now() / 1000) - 60,
			Math.floor(Date.now() / 1000) + 60,
		),
		growBy: new NumberRange(0.05, 0.05),
		labelStyle: {
			fontSize: 10,
		},
	});

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Left,
		autoRange: EAutoRange.Always,
		growBy: new NumberRange(0.15, 0.15),
		labelStyle: {
			fontSize: 10,
		},
	});

	sciChartSurface.xAxes.add(xAxis);
	sciChartSurface.yAxes.add(yAxis);

	const seriesByKind = new Map<PredictionSeriesKind, XyDataSeries>();

	for (const kind of ["actual", "prediction", "error"] as const) {
		const style = SERIES_STYLE[kind];
		const dataSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: style.name,
			dataIsSortedInX: true,
			dataEvenlySpacedInX: false,
			containsNaN: false,
			fifoCapacity: PREDICTION_FIFO_CAPACITY,
			capacity: PREDICTION_FIFO_CAPACITY,
		});

		sciChartSurface.renderableSeries.add(
			new FastLineRenderableSeries(wasmContext, {
				dataSeries,
				stroke: style.stroke,
				strokeThickness: style.strokeThickness,
				strokeDashArray: style.strokeDashArray,
			}),
		);
		seriesByKind.set(kind, dataSeries);
	}

	sciChartSurface.chartModifiers.add(
		new ZoomExtentsModifier({ modifierGroup: "chart" }),
		new MouseWheelZoomModifier({ modifierGroup: "chart" }),
		new ZoomPanModifier({ modifierGroup: "chart" }),
	);

	let horizonSec = 60;
	let rightEdge = Math.floor(Date.now() / 1000) + 60;

	const appendReading = (reading: PredictionReading) => {
		if (!Number.isFinite(reading.x) || !Number.isFinite(reading.value)) {
			return;
		}

		const dataSeries = seriesByKind.get(reading.kind);

		if (!dataSeries) {
			return;
		}

		const nativeX = dataSeries.getNativeXValues();
		const lastIndex = dataSeries.count() - 1;
		const priorLastX = lastIndex >= 0 ? nativeX.get(lastIndex) : null;

		if (priorLastX === reading.x) {
			dataSeries.update(lastIndex, reading.value);
		} else {
			dataSeries.append(reading.x, reading.value);
		}

		if (reading.horizon != null && reading.horizon > 0) {
			horizonSec = reading.horizon;
		}

		if (reading.kind === "prediction") {
			rightEdge = reading.x;
			xAxis.visibleRange = new NumberRange(
				rightEdge - 2 * horizonSec,
				rightEdge,
			);
		}

		sciChartSurface.invalidateElement();
	};

	return {
		sciChartSurface,
		appendReading,
	};
};
