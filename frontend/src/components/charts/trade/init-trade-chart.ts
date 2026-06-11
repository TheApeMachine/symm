import {
	DiscontinuousDateAxis,
	EAutoRange,
	EAxisAlignment,
	ENumericFormat,
	FastCandlestickRenderableSeries,
	FastColumnRenderableSeries,
	NumberRange,
	NumericAxis,
	OhlcDataSeries,
	SciChartSurface,
	Thickness,
} from "scichart";
import { SciTraderDarkTheme } from "scichart-financial-tools";
import { appTheme } from "./theme";

const Y_AXIS_VOLUME_ID = "Y_AXIS_VOLUME_ID";

export type TFinancialChartContext = {
	sciChartSurface: SciChartSurface;
	wasmContext: any;
	xAxis: DiscontinuousDateAxis;
	yAxis: NumericAxis;
	candlestickSeries: FastCandlestickRenderableSeries;
	xValues: number[];
	openValues: number[];
	highValues: number[];
	lowValues: number[];
	closeValues: number[];
	volumeValues: number[];
	xAt: (index: number) => number;
	yAt: (index: number, offsetFraction?: number) => number;
	priceAt: (index: number) => number;
	formatPrice: (value: number) => string;
};

export const initTradeChart = async (rootElement: string | HTMLDivElement) => {
	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{
			theme: new SciTraderDarkTheme(),
			padding: new Thickness(10, 10, 10, 10),
		},
	);

	const dataSeries = new OhlcDataSeries(wasmContext, {
		dataSeriesName: "BTC / USDT",
		dataIsSortedInX: true,
		dataEvenlySpacedInX: true,
		containsNaN: false,
	});

	const finiteNumber = (value: unknown): number | null =>
		typeof value === "number" && Number.isFinite(value) ? value : null;

	const addData = (frame: Record<string, unknown>) => {
		const sec = finiteNumber(frame.sec);
		const open = finiteNumber(frame.open);
		const high = finiteNumber(frame.high);
		const low = finiteNumber(frame.low);
		const close = finiteNumber(frame.close);

		if (
			sec === null ||
			open === null ||
			high === null ||
			low === null ||
			close === null
		) {
			return;
		}

		dataSeries.append(sec, open, high, low, close);
		sciChartSurface.zoomExtents(500);
	};

	const xAxis = new DiscontinuousDateAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Bottom,
		autoRange: EAutoRange.Never,
		cursorLabelFormat: ENumericFormat.Date_HHMM,
		drawMajorBands: false,
		drawMinorGridLines: false,
		majorGridLineStyle: { color: "#FFFFFF05" },
	});

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Right,
		growBy: new NumberRange(0.1, 0.2),
		labelFormat: ENumericFormat.Engineering,
		labelPrecision: 1,
		labelPrefix: "$",
		autoRange: EAutoRange.Always,
		drawMajorBands: false,
		drawMinorGridLines: false,
		majorGridLineStyle: { color: "#FFFFFF05" },
	});

	sciChartSurface.xAxes.add(xAxis);
	sciChartSurface.yAxes.add(yAxis);
	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			id: Y_AXIS_VOLUME_ID,
			axisAlignment: EAxisAlignment.Left,
			growBy: new NumberRange(0, 4),
			isVisible: false,
			autoRange: EAutoRange.Always,
		}),
	);

	const candlestickSeries = new FastCandlestickRenderableSeries(wasmContext, {
		id: "Candles",
		dataSeries: dataSeries,
		stroke: appTheme.ForegroundColor,
		strokeThickness: 1,
		dataPointWidth: 0.8,
		brushUp: `${appTheme.VividGreen}CC`,
		brushDown: `${appTheme.VividRed}CC`,
		strokeUp: appTheme.VividGreen,
		strokeDown: appTheme.VividRed,
	});

	sciChartSurface.renderableSeries.add(candlestickSeries);

	sciChartSurface.renderableSeries.add(
		new FastColumnRenderableSeries(wasmContext, {
			strokeThickness: 0,
			dataPointWidth: 0.65,
			yAxisId: Y_AXIS_VOLUME_ID,
		}),
	);

	return {
		sciChartSurface,
		wasmContext,
		addData,
	};
};
