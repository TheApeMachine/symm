import {
	EAutoRange,
	EAxisAlignment,
	ECoordinateMode,
	EHorizontalAnchorPoint,
	EVerticalAnchorPoint,
	HeatmapColorMap,
	NumberRange,
	NumericAxis,
	SciChartSurface,
	TextAnnotation,
	UniformHeatmapDataSeries,
	UniformHeatmapRenderableSeries,
	zeroArray2D,
} from "scichart";
import {
	normalizedSurpriseReading,
	parseResonanceUniverseFrame,
	type ResonanceSymbolSummary,
	type ResonanceUniverseFrame,
	sortedUniverseSymbols,
} from "#/components/charts/resonance/resonance-universe-frame";
import { recentHeatmapTimeRange } from "#/components/charts/resonance/resonance-xray-frame";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

export const UNIVERSE_HEATMAP_TIME_COLS = 120;
export const UNIVERSE_HEATMAP_TIME_WINDOW = 48;
export const UNIVERSE_HEATMAP_MAX_ROWS = 1024;

export const applyUniverseHeatmapRows = (
	zValues: number[][],
	historyBySymbol: Map<string, number[]>,
	symbols: ResonanceSymbolSummary[],
	surpriseScale: number,
): number => {
	const sortedSymbols = sortedUniverseSymbols(symbols).slice(
		0,
		UNIVERSE_HEATMAP_MAX_ROWS,
	);
	const activeRowCount = sortedSymbols.length;

	for (let rowIndex = 0; rowIndex < activeRowCount; rowIndex += 1) {
		const summary = sortedSymbols[rowIndex];
		const reading = normalizedSurpriseReading(summary.surprise, surpriseScale);
		let history = historyBySymbol.get(summary.symbol);

		if (history === undefined) {
			history = Array.from({ length: UNIVERSE_HEATMAP_TIME_COLS }, () => 0);
			historyBySymbol.set(summary.symbol, history);
		}

		for (
			let columnIndex = 0;
			columnIndex < UNIVERSE_HEATMAP_TIME_COLS - 1;
			columnIndex += 1
		) {
			history[columnIndex] = history[columnIndex + 1];
		}

		history[UNIVERSE_HEATMAP_TIME_COLS - 1] = reading;

		for (
			let columnIndex = 0;
			columnIndex < UNIVERSE_HEATMAP_TIME_COLS;
			columnIndex += 1
		) {
			zValues[rowIndex][columnIndex] = history[columnIndex] ?? 0;
		}
	}

	for (
		let rowIndex = activeRowCount;
		rowIndex < UNIVERSE_HEATMAP_MAX_ROWS;
		rowIndex += 1
	) {
		for (
			let columnIndex = 0;
			columnIndex < UNIVERSE_HEATMAP_TIME_COLS;
			columnIndex += 1
		) {
			zValues[rowIndex][columnIndex] = 0;
		}
	}

	return activeRowCount;
};

export const initResonanceUniverseHeatmap = async (
	rootElement: string | HTMLDivElement,
) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{
			background: appTheme.Background,
			freezeWhenOutOfView: false,
		},
	);

	const zValues = zeroArray2D([
		UNIVERSE_HEATMAP_MAX_ROWS,
		UNIVERSE_HEATMAP_TIME_COLS,
	]);
	const historyBySymbol = new Map<string, number[]>();

	sciChartSurface.xAxes.add(
		new NumericAxis(wasmContext, {
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(
				recentHeatmapTimeRange(
					UNIVERSE_HEATMAP_TIME_COLS,
					UNIVERSE_HEATMAP_TIME_WINDOW,
				).start,
				UNIVERSE_HEATMAP_TIME_COLS,
			),
		}),
	);

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Left,
		isVisible: false,
		autoRange: EAutoRange.Never,
		visibleRange: new NumberRange(-0.5, UNIVERSE_HEATMAP_MAX_ROWS - 0.5),
	});

	sciChartSurface.yAxes.add(yAxis);

	const xAxis = sciChartSurface.xAxes.get(0);

	const dataSeries = new UniformHeatmapDataSeries(wasmContext, {
		zValues,
		xStart: 0,
		xStep: 1,
		yStart: 0,
		yStep: 1,
	});

	const series = new UniformHeatmapRenderableSeries(wasmContext, {
		dataSeries,
		colorMap: new HeatmapColorMap({
			minimum: 0,
			maximum: 1,
			gradientStops: [
				{ offset: 0, color: "#0b0f14" },
				{ offset: 0.25, color: appTheme.Indigo },
				{ offset: 0.55, color: appTheme.VividOrange },
				{ offset: 1, color: appTheme.VividPink },
			],
		}),
		useLinearTextureFiltering: true,
	});

	sciChartSurface.renderableSeries.add(series);

	const headerAnnotation = new TextAnnotation({
		text: "Resonance universe",
		x1: 0,
		y1: 0,
		xCoordinateMode: ECoordinateMode.Relative,
		yCoordinateMode: ECoordinateMode.Relative,
		horizontalAnchorPoint: EHorizontalAnchorPoint.Left,
		verticalAnchorPoint: EVerticalAnchorPoint.Top,
		fontSize: 11,
		textColor: "rgba(226,232,240,0.85)",
		background: "rgba(11,15,20,0.65)",
	});

	sciChartSurface.annotations.add(headerAnnotation);

	let surpriseScale = 1;
	let activeRowCount = 0;

	const applyUniverseFrame = (frame: ResonanceUniverseFrame) => {
		surpriseScale = Math.max(
			surpriseScale * 0.98,
			...frame.symbols.map((entry) => entry.surprise),
			1e-6,
		);

		activeRowCount = applyUniverseHeatmapRows(
			zValues,
			historyBySymbol,
			frame.symbols,
			surpriseScale,
		);

		yAxis.visibleRange = new NumberRange(
			-0.5,
			Math.max(activeRowCount, 1) - 0.5,
		);

		const timeRange = recentHeatmapTimeRange(
			UNIVERSE_HEATMAP_TIME_COLS,
			UNIVERSE_HEATMAP_TIME_WINDOW,
		);

		xAxis.visibleRange = new NumberRange(timeRange.start, timeRange.end);
		dataSeries.setZValues(zValues);

		const cappedCount =
			frame.symbolCount > UNIVERSE_HEATMAP_MAX_ROWS
				? `${UNIVERSE_HEATMAP_MAX_ROWS}/${frame.symbolCount}`
				: `${frame.symbolCount}`;

		headerAnnotation.text = `${cappedCount} symbols · focus ${frame.focusSymbol} · ${frame.focus.category || "resonance"}`;
		sciChartSurface.invalidateElement();
	};

	const addData = (raw: Record<string, unknown>) => {
		if (sciChartSurface.isDeleted) {
			return;
		}

		const frame = parseResonanceUniverseFrame(raw);

		if (frame === null) {
			return;
		}

		applyUniverseFrame(frame);
	};

	return {
		sciChartSurface,
		wasmContext,
		addData,
	};
};
