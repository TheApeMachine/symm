import {
	EAutoRange,
	EColor,
	ENumericFormat,
	EPolarAxisMode,
	EPolarGridlineMode,
	EPolarLabelMode,
	NumberRange,
	PolarCategoryAxis,
	PolarMountainRenderableSeries,
	PolarNumericAxis,
	SciChartPolarSurface,
	XyDataSeries,
} from "scichart";
import { appTheme } from "#/components/charts/spider/theme";

export type SpiderControls = { update: (values: number[]) => void };

/*
drawSignalSpider renders a live radar of the current signal landscape: one axis
per signal, the radius is that signal's confidence (0-100). controls.update
replaces the petal values each frame to show the present shape of the market.
*/
export const drawSignalSpider = async (
	rootElement: string | HTMLDivElement,
	labels: string[],
) => {
	const { sciChartSurface, wasmContext } = await SciChartPolarSurface.create(
		rootElement,
		{ theme: appTheme.SciChartJsTheme },
	);

	const radialYAxis = new PolarNumericAxis(wasmContext, {
		polarAxisMode: EPolarAxisMode.Radial,
		gridlineMode: EPolarGridlineMode.Polygons,
		useNativeText: true,
		autoRange: EAutoRange.Never,
		visibleRange: new NumberRange(0, 100),
		majorGridLineStyle: {
			color: EColor.BackgroundColor,
			strokeThickness: 1,
			strokeDashArray: [5, 5],
		},
		drawLabels: false,
		drawMinorGridLines: false,
		drawMajorTickLines: false,
		drawMinorTickLines: false,
		startAngle: Math.PI / 2,
		innerRadius: 0,
	});
	sciChartSurface.yAxes.add(radialYAxis);

	const angularXAxis = new PolarCategoryAxis(wasmContext, {
		polarAxisMode: EPolarAxisMode.Angular,
		labels,
		majorGridLineStyle: {
			color: EColor.BackgroundColor,
			strokeThickness: 1,
			strokeDashArray: [5, 5],
		},
		flippedCoordinates: true,
		drawMinorGridLines: false,
		useNativeText: true,
		polarLabelMode: EPolarLabelMode.Horizontal,
		labelFormat: ENumericFormat.NoFormat,
		startAngle: Math.PI / 2,
	});
	sciChartSurface.xAxes.add(angularXAxis);

	// +1 closes the loop so the first and last petal join without overlap.
	const xValues = Array.from({ length: labels.length + 1 }, (_, index) => index);
	const dataSeries = new XyDataSeries(wasmContext, {
		xValues,
		yValues: new Array(labels.length + 1).fill(0),
		dataSeriesName: "Confidence",
	});

	sciChartSurface.renderableSeries.add(
		new PolarMountainRenderableSeries(wasmContext, {
			dataSeries,
			stroke: appTheme.VividSkyBlue,
			fill: `${appTheme.VividSkyBlue}30`,
			strokeThickness: 3,
		}),
	);

	sciChartSurface.background = "transparent";

	return {
		sciChartSurface,
		wasmContext,
		controls: {
			update(values: number[]) {
				if (sciChartSurface.isDeleted) {
					return;
				}

				const closed = [...values, values[0] ?? 0];

				dataSeries.clear();
				dataSeries.appendRange(xValues, closed);
				sciChartSurface.invalidateElement();
			},
		} satisfies SpiderControls,
	};
};
