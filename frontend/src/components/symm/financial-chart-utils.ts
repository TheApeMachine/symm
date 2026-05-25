import {
	AnnotationHoverModifier,
	DiscontinuousDateAxis,
	EAutoRange,
	EAxisAlignment,
	EBaseType,
	ECursorStyle,
	EFillPaletteMode,
	ENumericFormat,
	EPaletteProviderType,
	EXyDirection,
	FastCandlestickRenderableSeries,
	FastColumnRenderableSeries,
	type IFillPaletteProvider,
	type IPointMetadata,
	type IRenderableSeries,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	OhlcDataSeries,
	parseColorToUIntArgb,
	registerType,
	SciChartSurface,
	type TPaletteProviderDefinition,
	Thickness,
	XyDataSeries,
	ZoomExtentsModifier,
	type TSciChart,
} from "scichart";
import { SciTraderDarkTheme } from "scichart-financial-tools";

export const Y_AXIS_VOLUME_ID = "Y_AXIS_VOLUME_ID";
export const VISIBLE_CANDLE_COUNT = 300;

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
				: 60;
	const pad = Math.max(barStep * 2, 60);

	return { min: firstX - pad, max: lastX + pad };
};

export const shiftTrailingVisibleRange = (
	visibleMin: number,
	visibleMax: number,
	lastX: number,
	barStep: number,
): { min: number; max: number } => {
	const followTolerance = Math.max(barStep * 2, 60);
	const span = visibleMax - visibleMin;
	const pad = followTolerance / 2;

	return { min: lastX + pad - span, max: lastX + pad };
};

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
	const barStep = lastIndex > 0 ? lastX - nativeX.get(lastIndex - 1) : 60;

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

const VOLUME_PALETTE_PROVIDER_TYPE = "TradingAnnotationVolumePaletteProvider";

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
	openValues: number[];
	closeValues: number[];
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
		containsNaN: false,
	});

	const volume = new XyDataSeries(wasmContext, {
		dataSeriesName: "Volume",
		dataIsSortedInX: true,
		containsNaN: false,
	});

	const openValues: number[] = [];
	const closeValues: number[] = [];

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
			openValues,
			closeValues,
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
		openValues,
		closeValues,
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

	const nativeHigh = ohlc.getNativeHighValues();
	const nativeLow = ohlc.getNativeLowValues();
	let minLow = Number.POSITIVE_INFINITY;
	let maxHigh = Number.NEGATIVE_INFINITY;

	for (let index = 0; index < barCount; index++) {
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

class VolumePaletteProvider implements IFillPaletteProvider {
	public readonly fillPaletteMode: EFillPaletteMode = EFillPaletteMode.SOLID;
	private readonly openValues: number[];
	private readonly closeValues: number[];
	private readonly upColorArgb: number;
	private readonly downColorArgb: number;

	constructor(
		openValues: number[],
		closeValues: number[],
		upColor: string,
		downColor: string,
	) {
		this.openValues = openValues;
		this.closeValues = closeValues;
		this.upColorArgb = parseColorToUIntArgb(upColor);
		this.downColorArgb = parseColorToUIntArgb(downColor);
	}

	public onAttached(_parentSeries: IRenderableSeries): void {}

	public onDetached(): void {}

	public overrideFillArgb(
		_xValue: number,
		_yValue: number,
		index: number,
		_opacity?: number,
		_metadata?: IPointMetadata,
	): number {
		const open = this.openValues[index];
		const close = this.closeValues[index] ?? open;

		if (close >= open) {
			return this.upColorArgb;
		}

		return this.downColorArgb;
	}

	public overrideStrokeArgb(
		xValue: number,
		yValue: number,
		index: number,
		opacity?: number,
		metadata?: IPointMetadata,
	): number {
		return this.overrideFillArgb(xValue, yValue, index, opacity, metadata);
	}

	public toJSON(): TPaletteProviderDefinition {
		const isUpByIndex = this.openValues.map(
			(open, index) => (this.closeValues[index] ?? open) >= open,
		);

		return {
			type: EPaletteProviderType.Custom,
			customType: VOLUME_PALETTE_PROVIDER_TYPE,
			options: {
				isUpByIndex,
				upColor: `${VIVID_GREEN}66`,
				downColor: `${VIVID_RED}66`,
			},
		};
	}
}

registerType(
	EBaseType.PaletteProvider,
	VOLUME_PALETTE_PROVIDER_TYPE,
	(options?: { isUpByIndex?: boolean[] }) => {
		const isUpByIndex = options?.isUpByIndex ?? [];
		const openValues = isUpByIndex.map((isUp) => (isUp ? 0 : 1));
		const closeValues = isUpByIndex.map((isUp) => (isUp ? 1 : 0));

		return new VolumePaletteProvider(
			openValues,
			closeValues,
			`${VIVID_GREEN}66`,
			`${VIVID_RED}66`,
		);
	},
	true,
);
