import {
	SciChartSurface,
	NumericAxis,
	EAxisAlignment,
	EAutoRange,
	ENumericFormat,
	NumberRange,
	XyDataSeries,
	FastLineRenderableSeries,
	ZoomExtentsModifier,
	MouseWheelZoomModifier,
	ZoomPanModifier,
} from "scichart";

import type {
	PredictionReading,
	PredictionSeriesKind,
} from "#/components/symm/predictions-data-provider";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const SERIES_STYLE: Record<
	PredictionSeriesKind,
	{
		name: string;
		stroke: string;
		strokeDashArray?: number[];
		strokeThickness: number;
	}
> = {
	average: {
		name: "Realized cross-section multiple",
		stroke: "#4EC385",
		strokeThickness: 2,
	},
	prediction: {
		name: "Forward forecast (next pulse)",
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

export type PredictionsChartControls = {
	appendReading: (reading: PredictionReading) => void;
};

export const drawExample = async (rootElement: string | HTMLDivElement) => {
	await ensureSciChartWasm();

	const { wasmContext, sciChartSurface } =
		await SciChartSurface.create(rootElement);

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

	for (const kind of ["average", "prediction", "error"] as const) {
		const style = SERIES_STYLE[kind];
		const dataSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: style.name,
			containsNaN: false,
			isSorted: false,
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

	let minX = Number.POSITIVE_INFINITY;
	let maxX = Number.NEGATIVE_INFINITY;

	const appendReading = (reading: PredictionReading) => {
		const dataSeries = seriesByKind.get(reading.kind);

		if (!dataSeries) {
			return;
		}

		dataSeries.append(reading.x, reading.value);
		sciChartSurface.invalidateElement();

		if (!Number.isFinite(reading.x) || !Number.isFinite(reading.value)) {
			return;
		}

		minX = Math.min(minX, reading.x);
		maxX = Math.max(maxX, reading.x);

		if (Number.isFinite(minX) && Number.isFinite(maxX)) {
			const pad = Math.max(2, (maxX - minX) * 0.05);

			xAxis.visibleRange = new NumberRange(minX - pad, maxX + pad);
		}
	};

	return {
		wasmContext,
		sciChartSurface,
		controls: { appendReading },
	};
};
