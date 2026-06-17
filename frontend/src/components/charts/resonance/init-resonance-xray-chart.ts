import {
	EAutoRange,
	EAxisAlignment,
	ECoordinateMode,
	EHorizontalAnchorPoint,
	EVerticalAnchorPoint,
	FastColumnRenderableSeries,
	HeatmapColorMap,
	NumberRange,
	NumericAxis,
	Rect,
	SciChartSubSurface,
	SciChartSurface,
	TextAnnotation,
	Thickness,
	UniformHeatmapDataSeries,
	UniformHeatmapRenderableSeries,
	XyDataSeries,
	zeroArray2D,
} from "scichart";
import { parseResonanceUniverseFrame } from "#/components/charts/resonance/resonance-universe-frame";
import {
	flattenResonanceStates,
	type ResonanceLayerFrame,
	recentHeatmapTimeRange,
	resonanceChannelLabels,
	shiftHeatmapRow,
} from "#/components/charts/resonance/resonance-xray-frame";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

const TIME_COLS = 160;
const TIME_WINDOW_COLS = 64;
const MAX_STATE_ROWS = 32;
const MAX_ERROR_ROWS = 8;

const STATE_RECT = new Rect(0, 0, 1, 0.56);
const ERROR_RECT = new Rect(0, 0.56, 1, 0.26);
const CROSS_RECT = new Rect(0, 0.82, 1, 0.18);

const activationColorMap = () =>
	new HeatmapColorMap({
		minimum: -1,
		maximum: 1,
		gradientStops: [
			{ offset: 0, color: appTheme.VividBlue },
			{ offset: 0.45, color: "#0b0f14" },
			{ offset: 0.55, color: "#0b0f14" },
			{ offset: 1, color: appTheme.VividPink },
		],
	});

const errorColorMap = () =>
	new HeatmapColorMap({
		minimum: 0,
		maximum: 1,
		gradientStops: [
			{ offset: 0, color: "#0b0f14" },
			{ offset: 0.35, color: appTheme.Indigo },
			{ offset: 0.7, color: appTheme.VividOrange },
			{ offset: 1, color: appTheme.VividRed },
		],
	});

const createHeatmapSubChart = (
	parentSurface: SciChartSurface,
	position: Rect,
	rowCount: number,
	colorMap: HeatmapColorMap,
) => {
	const subChart = SciChartSubSurface.createSubSurface(parentSurface, {
		position,
		padding: new Thickness(8, 8, 8, 8),
		background: appTheme.Background,
	});

	const wasmContext = subChart.webAssemblyContext2D;
	const zValues = zeroArray2D([rowCount, TIME_COLS]);
	const timeRange = recentHeatmapTimeRange(TIME_COLS, TIME_WINDOW_COLS);

	const xAxis = new NumericAxis(wasmContext, {
		isVisible: false,
		autoRange: EAutoRange.Never,
		visibleRange: new NumberRange(timeRange.start, timeRange.end),
	});

	subChart.xAxes.add(xAxis);

	const yAxis = new NumericAxis(wasmContext, {
		axisAlignment: EAxisAlignment.Left,
		isVisible: false,
		autoRange: EAutoRange.Never,
		visibleRange: new NumberRange(-0.5, rowCount - 0.5),
	});

	subChart.yAxes.add(yAxis);

	const dataSeries = new UniformHeatmapDataSeries(wasmContext, {
		zValues,
		xStart: 0,
		xStep: 1,
		yStart: 0,
		yStep: 1,
	});

	const series = new UniformHeatmapRenderableSeries(wasmContext, {
		dataSeries,
		colorMap,
		useLinearTextureFiltering: true,
	});

	subChart.renderableSeries.add(series);

	return {
		subChart,
		xAxis,
		yAxis,
		zValues,
		dataSeries,
		series,
	};
};

const createCrossSectionSubChart = (parentSurface: SciChartSurface) => {
	const subChart = SciChartSubSurface.createSubSurface(parentSurface, {
		position: CROSS_RECT,
		padding: new Thickness(8, 8, 4, 48),
		background: appTheme.Background,
	});

	const wasmContext = subChart.webAssemblyContext2D;

	subChart.xAxes.add(
		new NumericAxis(wasmContext, {
			autoRange: EAutoRange.Always,
			drawMajorGridLines: false,
			drawMinorGridLines: false,
		}),
	);

	subChart.yAxes.add(
		new NumericAxis(wasmContext, {
			autoRange: EAutoRange.Always,
			drawMajorGridLines: false,
			drawMinorGridLines: false,
		}),
	);

	const observationSeries = new XyDataSeries(wasmContext, {
		containsNaN: false,
		isSorted: true,
	});

	const predictionSeries = new XyDataSeries(wasmContext, {
		containsNaN: false,
		isSorted: true,
	});

	subChart.renderableSeries.add(
		new FastColumnRenderableSeries(wasmContext, {
			dataSeries: observationSeries,
			fill: `${appTheme.VividSkyBlue}CC`,
			strokeThickness: 0,
			dataPointWidth: 0.35,
		}),
		new FastColumnRenderableSeries(wasmContext, {
			dataSeries: predictionSeries,
			fill: `${appTheme.VividOrange}AA`,
			strokeThickness: 0,
			dataPointWidth: 0.35,
		}),
	);

	return {
		subChart,
		observationSeries,
		predictionSeries,
	};
};

const updateCrossSection = (
	observationSeries: XyDataSeries,
	predictionSeries: XyDataSeries,
	inputLayer: ResonanceLayerFrame | undefined,
) => {
	observationSeries.clear();
	predictionSeries.clear();

	if (inputLayer === undefined) {
		return;
	}

	for (
		let featureIndex = 0;
		featureIndex < inputLayer.state.length;
		featureIndex += 1
	) {
		const xPosition = featureIndex * 2;
		observationSeries.append(xPosition, inputLayer.state[featureIndex] ?? 0);
		predictionSeries.append(
			xPosition + 0.65,
			inputLayer.prediction[featureIndex] ?? 0,
		);
	}
};

const replaceChannelLabels = (
	subChart: SciChartSubSurface,
	labels: string[],
	labelAnnotations: TextAnnotation[],
) => {
	for (const annotation of labelAnnotations) {
		subChart.annotations.remove(annotation);
	}

	labelAnnotations.length = 0;

	for (let rowIndex = 0; rowIndex < labels.length; rowIndex += 1) {
		const annotation = new TextAnnotation({
			text: labels[rowIndex] ?? "",
			x1: 0.01,
			y1: rowIndex,
			xCoordinateMode: ECoordinateMode.Relative,
			yCoordinateMode: ECoordinateMode.DataValue,
			horizontalAnchorPoint: EHorizontalAnchorPoint.Left,
			verticalAnchorPoint: EVerticalAnchorPoint.Center,
			fontSize: 9,
			textColor: "rgba(226,232,240,0.75)",
			background: "rgba(11,15,20,0.55)",
		});

		subChart.annotations.add(annotation);
		labelAnnotations.push(annotation);
	}
};

export const initResonanceXRayChart = async (
	rootElement: string | HTMLDivElement,
) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{
			padding: new Thickness(0, 0, 0, 0),
			background: appTheme.Background,
			freezeWhenOutOfView: false,
		},
	);

	const stateHeatmap = createHeatmapSubChart(
		sciChartSurface,
		STATE_RECT,
		MAX_STATE_ROWS,
		activationColorMap(),
	);

	const errorHeatmap = createHeatmapSubChart(
		sciChartSurface,
		ERROR_RECT,
		MAX_ERROR_ROWS,
		errorColorMap(),
	);

	const crossSection = createCrossSectionSubChart(sciChartSurface);

	sciChartSurface.xAxes.add(
		new NumericAxis(wasmContext, {
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(0, 1),
		}),
	);

	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(0, 1),
		}),
	);

	const headerAnnotation = new TextAnnotation({
		text: "Resonance X-Ray",
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

	const channelLabelAnnotations: TextAnnotation[] = [];
	let errorScale = 1;
	let stateScale = 1;

	const addData = (raw: Record<string, unknown>) => {
		if (sciChartSurface.isDeleted) {
			return;
		}

		const universe = parseResonanceUniverseFrame(raw);

		if (universe === null) {
			return;
		}

		const frame = universe.focus;
		const flattened = flattenResonanceStates(frame.layers);
		const activeStateRows = Math.min(flattened.length, MAX_STATE_ROWS);

		stateScale = Math.max(
			stateScale * 0.95,
			...flattened.map((value) => Math.abs(value)),
			1e-6,
		);

		for (let rowIndex = 0; rowIndex < activeStateRows; rowIndex += 1) {
			const normalizedState = Math.min(
				1,
				Math.max(-1, (flattened[rowIndex] ?? 0) / stateScale),
			);

			shiftHeatmapRow(
				stateHeatmap.zValues[rowIndex],
				normalizedState,
				TIME_COLS,
			);
		}

		const activeErrorRows = Math.min(frame.layers.length, MAX_ERROR_ROWS);
		errorScale = Math.max(
			errorScale * 0.95,
			frame.surprise,
			...frame.layers.map((layer) => layer.errorNorm),
			1e-6,
		);

		for (let rowIndex = 0; rowIndex < activeErrorRows; rowIndex += 1) {
			const normalizedError =
				(frame.layers[rowIndex]?.errorNorm ?? 0) / errorScale;

			shiftHeatmapRow(
				errorHeatmap.zValues[rowIndex],
				Math.min(1, Math.max(0, normalizedError)),
				TIME_COLS,
			);
		}

		const timeRange = recentHeatmapTimeRange(TIME_COLS, TIME_WINDOW_COLS);

		stateHeatmap.xAxis.visibleRange = new NumberRange(
			timeRange.start,
			timeRange.end,
		);
		errorHeatmap.xAxis.visibleRange = new NumberRange(
			timeRange.start,
			timeRange.end,
		);
		stateHeatmap.yAxis.visibleRange = new NumberRange(
			-0.5,
			Math.max(activeStateRows, 1) - 0.5,
		);
		errorHeatmap.yAxis.visibleRange = new NumberRange(
			-0.5,
			Math.max(activeErrorRows, 1) - 0.5,
		);

		stateHeatmap.dataSeries.setZValues(stateHeatmap.zValues);
		errorHeatmap.dataSeries.setZValues(errorHeatmap.zValues);
		errorHeatmap.series.colorMap.maximum = 1;

		replaceChannelLabels(
			stateHeatmap.subChart,
			resonanceChannelLabels(frame.layers).slice(0, activeStateRows),
			channelLabelAnnotations,
		);

		updateCrossSection(
			crossSection.observationSeries,
			crossSection.predictionSeries,
			frame.layers[0],
		);

		headerAnnotation.text = `${frame.symbol} · focus x-ray · ${frame.category || "resonance"} · surprise ${frame.surprise.toFixed(4)} · energy ${frame.energy.toFixed(4)} · conf ${(frame.confidence * 100).toFixed(1)}%`;

		sciChartSurface.invalidateElement();
	};

	return {
		sciChartSurface,
		wasmContext,
		addData,
	};
};
