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
import { ensureSciChartWasm } from "#/lib/utils";

export type SpiderControls = { update: (values: number[]) => void };

/*
drawSignalSpider renders a live radar of cross-section mean regime strengths:
volatility, trend, bullish, bearish, and choppiness on a 0-100 radius.
*/
export const drawSignalSpider = async (
	rootElement: string | HTMLDivElement,
	labels: string[],
) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartPolarSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
			freezeWhenOutOfView: true,
		},
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
	const xValues = Array.from(
		{ length: labels.length + 1 },
		(_, index) => index,
	);
	const dataSeries = new XyDataSeries(wasmContext, {
		xValues,
		yValues: new Array(labels.length + 1).fill(0),
		dataSeriesName: "Confidence",
		dataIsSortedInX: true,
		dataEvenlySpacedInX: true,
		containsNaN: false,
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
				const nativeY = dataSeries.getNativeYValues();

				for (let index = 0; index < closed.length; index++) {
					nativeY.set(index, closed[index]);
				}

				sciChartSurface.invalidateElement();
			},
		} satisfies SpiderControls,
	};
};
