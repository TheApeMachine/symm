import {
	SciChartSurface,
	NumericAxis,
	EAxisAlignment,
	NumberRange,
	XyDataSeries,
	XyScatterRenderableSeries,
	EllipsePointMarker,
	ZoomExtentsModifier,
	MouseWheelZoomModifier,
	ZoomPanModifier,
	RolloverModifier,
} from "scichart";

import type {
	PredictionReading,
	PredictionSeriesKind,
} from "#/components/symm/predictions-data-provider";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const SERIES_STYLE: Record<
	PredictionSeriesKind,
	{ name: string; stroke: string; fill: string; width: number }
> = {
	predicted: {
		name: "Predicted",
		stroke: "#FBA55A",
		fill: "rgba(251, 165, 90, 0.35)",
		width: 9,
	},
	actual: {
		name: "Actual",
		stroke: "#4EC385",
		fill: "rgba(78, 195, 133, 0.35)",
		width: 8,
	},
	error: {
		name: "Error",
		stroke: "#E85D75",
		fill: "rgba(232, 93, 117, 0.35)",
		width: 7,
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
		axisTitle: "Time (UTC s)",
		growBy: new NumberRange(0.05, 0.05),
	});

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Left,
		axisTitle: "Return %",
		growBy: new NumberRange(0.15, 0.15),
	});

	sciChartSurface.xAxes.add(xAxis);
	sciChartSurface.yAxes.add(yAxis);

	const seriesByKind = new Map<PredictionSeriesKind, XyDataSeries>();

	for (const kind of ["predicted", "actual", "error"] as const) {
		const style = SERIES_STYLE[kind];
		const dataSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: style.name,
			containsNaN: false,
		});

		sciChartSurface.renderableSeries.add(
			new XyScatterRenderableSeries(wasmContext, {
				dataSeries,
				stroke: style.stroke,
				fill: style.fill,
				pointMarker: new EllipsePointMarker(wasmContext, {
					width: style.width,
					height: style.width,
					strokeThickness: 2,
					fill: style.fill,
					stroke: style.stroke,
				}),
			}),
		);
		seriesByKind.set(kind, dataSeries);
	}

	sciChartSurface.chartModifiers.add(
		new ZoomExtentsModifier({ modifierGroup: "chart" }),
		new MouseWheelZoomModifier({ modifierGroup: "chart" }),
		new ZoomPanModifier({ modifierGroup: "chart" }),
		new RolloverModifier({ modifierGroup: "chart" }),
	);

	sciChartSurface.zoomExtents();

	let zoomed = false;

	const appendReading = (reading: PredictionReading) => {
		const dataSeries = seriesByKind.get(reading.kind);

		if (!dataSeries) {
			return;
		}

		dataSeries.append(reading.x, reading.value);
		sciChartSurface.invalidateElement();

		if (!zoomed) {
			sciChartSurface.zoomExtents();
			zoomed = true;
		}
	};

	return {
		wasmContext,
		sciChartSurface,
		controls: { appendReading },
	};
};
