import {
	AnnotationHoverModifier,
	DiscontinuousDateAxis,
	EAutoRange,
	EAxisAlignment,
	ECursorStyle,
	ENumericFormat,
	EXyDirection,
	FastCandlestickRenderableSeries,
	FastColumnRenderableSeries,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	OhlcDataSeries,
	SciChartSurface,
	Thickness,
	type TSciChart,
	XyDataSeries,
	ZoomExtentsModifier,
} from "scichart";
import { SciTraderDarkTheme } from "scichart-financial-tools";

import { VolumePaletteProvider } from "#/components/charts/shared/volume-palette-provider";

export const Y_AXIS_VOLUME_ID = "Y_AXIS_VOLUME_ID";
export const VISIBLE_CANDLE_COUNT = 300;
const TRADE_FIFO_CAPACITY = VISIBLE_CANDLE_COUNT * 2;
const DEFAULT_BAR_STEP_SEC = 60;
const DEFAULT_PAD_SEC = 60;
export const DEFAULT_RANGE_MATCH_EPSILON_SEC = 1;

export const candleChartXExtents = (
	firstX: number,
	lastX: number,
	barCountInWindow: number,
	priorBarX?: number,
): { min: number; max: number } => {
	const barStep =
		barCountInWindow > 1 && lastX > firstX
			? (lastX - firstX) / (barCountInWindow - 1)
			: priorBarX !== undefined
				? lastX - priorBarX
				: DEFAULT_BAR_STEP_SEC;
	const pad = Math.max(barStep * 2, DEFAULT_PAD_SEC);

	return { min: firstX - pad, max: lastX + pad };
};

export const shiftTrailingVisibleRange = (
	visibleMin: number,
	visibleMax: number,
	lastX: number,
	barStep: number,
): { min: number; max: number } => {
	const followTolerance = Math.max(barStep * 2, DEFAULT_PAD_SEC);
	const span = visibleMax - visibleMin;
	const pad = followTolerance / 2;

	return { min: lastX + pad - span, max: lastX + pad };
};

/**
 * Returns the live-follow tolerance in seconds.
 *
 * @param barStep Bar spacing in seconds.
 * @returns The greater of barStep * 2 and DEFAULT_PAD_SEC, in seconds.
 */
export const liveFollowToleranceSec = (barStep: number): number =>
	Math.max(barStep * 2, DEFAULT_PAD_SEC);

/**
 * Reports whether the current viewport is still tracking the live edge.
 *
 * @param currentRange Visible x-axis range in seconds; currentRange.max is the visible right edge.
 * @param priorLastX Previous final bar x value in seconds.
 * @param barStep Bar spacing in seconds.
 * @returns True when currentRange.max is within liveFollowToleranceSec(barStep) of priorLastX.
 */
export const isViewportFollowingLiveEdge = (
	currentRange: NumberRange,
	priorLastX: number,
	barStep: number,
): boolean => {
	const followTolerance = liveFollowToleranceSec(barStep);

	return currentRange.max >= priorLastX - followTolerance;
};

export const visibleRangesMatch = (
	left: NumberRange,
	right: NumberRange,
	epsilon = DEFAULT_RANGE_MATCH_EPSILON_SEC,
): boolean =>
	Math.abs(left.min - right.min) <= epsilon &&
	Math.abs(left.max - right.max) <= epsilon;

export const resolveFollowVisibleRange = (
	ohlc: OhlcDataSeries,
	mode: "initial" | "live",
	currentRange?: NumberRange,
): NumberRange | null => {
	const barCount = ohlc.count();

	if (barCount <= 0) {
		return null;
	}

	const nativeX = ohlc.getNativeXValues();
	const lastIndex = barCount - 1;
	const lastX = nativeX.get(lastIndex);
	const barStep =
		lastIndex > 0 ? lastX - nativeX.get(lastIndex - 1) : DEFAULT_BAR_STEP_SEC;

	if (mode === "live" && currentRange !== undefined) {
		const shifted = shiftTrailingVisibleRange(
			currentRange.min,
			currentRange.max,
			lastX,
			barStep,
		);

		return new NumberRange(shifted.min, shifted.max);
	}

	const firstIndex = Math.max(0, lastIndex - VISIBLE_CANDLE_COUNT + 1);
	const firstX = nativeX.get(firstIndex);
	const priorBarX = lastIndex > 0 ? nativeX.get(lastIndex - 1) : undefined;
	const { min, max } = candleChartXExtents(
		firstX,
		lastX,
		lastIndex - firstIndex + 1,
		priorBarX,
	);

	return new NumberRange(min, max);
};

const FOREGROUND_COLOR = "#F5F5F5";
const VIVID_GREEN = "#67BDAF";
const VIVID_RED = "#C52E60";

export type FinancialChartContext = {
	sciChartSurface: SciChartSurface;
	wasmContext: TSciChart;
	xAxis: DiscontinuousDateAxis;
	yAxis: NumericAxis;
	candlestickSeries: FastCandlestickRenderableSeries;
	volumeSeries: FastColumnRenderableSeries;
	ohlc: OhlcDataSeries;
	volume: XyDataSeries;
};

export const createFinancialChartSurface = async (
	rootElement: HTMLDivElement,
	title: string,
): Promise<FinancialChartContext> => {
	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{
			theme: new SciTraderDarkTheme(),
			padding: new Thickness(10, 10, 10, 10),
			freezeWhenOutOfView: true,
		},
	);

	const xAxis = new DiscontinuousDateAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Bottom,
		autoRange: EAutoRange.Never,
		cursorLabelFormat: ENumericFormat.Date_HHMM,
		drawMajorBands: false,
		drawMinorGridLines: false,
	});

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Right,
		growBy: new NumberRange(0.1, 0.2),
		labelFormat: ENumericFormat.Decimal,
		labelPrefix: "$",
		autoRange: EAutoRange.Always,
		drawMajorBands: false,
		drawMinorGridLines: false,
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

	const ohlc = new OhlcDataSeries(wasmContext, {
		dataSeriesName: title,
		dataIsSortedInX: true,
		dataEvenlySpacedInX: true,
		containsNaN: false,
		fifoCapacity: TRADE_FIFO_CAPACITY,
		capacity: TRADE_FIFO_CAPACITY,
	});

	const volume = new XyDataSeries(wasmContext, {
		dataSeriesName: "Volume",
		dataIsSortedInX: true,
		dataEvenlySpacedInX: true,
		containsNaN: false,
		fifoCapacity: TRADE_FIFO_CAPACITY,
		capacity: TRADE_FIFO_CAPACITY,
	});

	const candlestickSeries = new FastCandlestickRenderableSeries(wasmContext, {
		id: "Candles",
		dataSeries: ohlc,
		stroke: FOREGROUND_COLOR,
		strokeThickness: 1,
		dataPointWidth: 0.8,
		brushUp: `${VIVID_GREEN}CC`,
		brushDown: `${VIVID_RED}CC`,
		strokeUp: VIVID_GREEN,
		strokeDown: VIVID_RED,
	});

	const volumeSeries = new FastColumnRenderableSeries(wasmContext, {
		dataSeries: volume,
		strokeThickness: 0,
		dataPointWidth: 0.65,
		yAxisId: Y_AXIS_VOLUME_ID,
		paletteProvider: new VolumePaletteProvider(
			ohlc,
			`${VIVID_GREEN}66`,
			`${VIVID_RED}66`,
		),
	});

	sciChartSurface.renderableSeries.add(candlestickSeries, volumeSeries);
	addDefaultFinancialModifiers(sciChartSurface);
	configureFinancialPriceAxis(yAxis, ohlc);

	return {
		sciChartSurface,
		wasmContext,
		xAxis,
		yAxis,
		candlestickSeries,
		volumeSeries,
		ohlc,
		volume,
	};
};

export const addDefaultFinancialModifiers = (
	sciChartSurface: SciChartSurface,
) => {
	sciChartSurface.chartModifiers.add(
		new MouseWheelZoomModifier({ xyDirection: EXyDirection.XDirection }),
		new ZoomExtentsModifier({ xyDirection: EXyDirection.XDirection }),
		new AnnotationHoverModifier({
			enableHover: true,
			enableCursor: true,
			idleCursor: ECursorStyle.Crosshair,
		}),
	);
};

export const followLatestCandleRange = (
	xAxis: DiscontinuousDateAxis,
	ohlc: OhlcDataSeries,
	mode: "initial" | "live" = "live",
) => {
	const nextRange = resolveFollowVisibleRange(ohlc, mode, xAxis.visibleRange);

	if (nextRange === null) {
		return;
	}

	xAxis.visibleRange = nextRange;
};

export const refreshFinancialPriceAxis = (
	yAxis: NumericAxis,
	ohlc: OhlcDataSeries,
) => {
	const barCount = ohlc.count();

	if (barCount <= 0) {
		return;
	}

	// Scan only the VISIBLE window: this runs on every candle update
	// (including in-place updates of the last bar), so a full-history scan was
	// O(n) per tick and O(n²) over a session. The label precision only ever
	// reflects what is on screen anyway — the chart follows the tail.
	const nativeHigh = ohlc.getNativeHighValues();
	const nativeLow = ohlc.getNativeLowValues();
	const firstIndex = Math.max(0, barCount - VISIBLE_CANDLE_COUNT);
	let minLow = Number.POSITIVE_INFINITY;
	let maxHigh = Number.NEGATIVE_INFINITY;

	for (let index = firstIndex; index < barCount; index++) {
		minLow = Math.min(minLow, nativeLow.get(index));
		maxHigh = Math.max(maxHigh, nativeHigh.get(index));
	}

	const span = Math.max(maxHigh - minLow, maxHigh * 1e-8);
	const labelDecimals = priceLabelDecimals(span);
	const cursorDecimals = labelDecimals + 1;
	const formatPrice = (value: number) => `$${value.toFixed(labelDecimals)}`;

	yAxis.labelProvider.formatLabel = formatPrice;
	yAxis.labelProvider.formatCursorLabel = (value: number) =>
		`$${value.toFixed(cursorDecimals)}`;
};

export const priceLabelDecimals = (span: number): number => {
	if (span >= 1000) {
		return 0;
	}

	if (span >= 100) {
		return 1;
	}

	if (span >= 10) {
		return 2;
	}

	if (span >= 1) {
		return 3;
	}

	if (span >= 0.01) {
		return 4;
	}

	return 6;
};

const configureFinancialPriceAxis = (
	yAxis: NumericAxis,
	ohlc: OhlcDataSeries,
) => {
	refreshFinancialPriceAxis(yAxis, ohlc);
};
