import { memo, useCallback } from "react";
import {
	EAutoRange,
	EAxisAlignment,
	ECoordinateMode,
	EHorizontalAnchorPoint,
	EVerticalAnchorPoint,
	HeatmapColorMap,
	NumericAxis,
	NumberRange,
	SciChartSurface,
	TextAnnotation,
	UniformHeatmapDataSeries,
	UniformHeatmapRenderableSeries,
	zeroArray2D,
} from "scichart";
import { SciChartReact } from "scichart-react";

import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const SOURCES = [
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
	"depthflow",
	"leadlag",
	"liquidity",
	"sentiment",
] as const;

const SOURCE_LABELS: Record<string, string> = {
	hawkes: "Hawkes",
	fluid: "Fluid",
	pumpdump: "Pump",
	causal: "Causal",
	depthflow: "Depth",
	leadlag: "L-Lag",
	liquidity: "Basis",
	sentiment: "Sent",
};

// Rolling window: how many time columns to keep
const TIME_COLS = 120;
// Publish a new column every 500ms
const TICK_MS = 500;

const initSignalHeatmap = async (rootElement: string | HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
	);

	const nRows = SOURCES.length;
	// zValues[row][col]: row = source index, col = time step (oldest → newest)
	const zValues = zeroArray2D([nRows, TIME_COLS]);

	sciChartSurface.xAxes.add(
		new NumericAxis(wasmContext, {
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(0, TIME_COLS),
		}),
	);

	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			axisAlignment: EAxisAlignment.Left,
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(-0.5, nRows - 0.5),
		}),
	);

	const dataSeries = new UniformHeatmapDataSeries(wasmContext, {
		zValues,
		xStart: 0,
		xStep: 1,
		yStart: 0,
		yStep: 1,
	});

	const colorMap = new HeatmapColorMap({
		minimum: 0,
		maximum: 4,
		gradientStops: [
			{ offset: 0, color: "#0b0f14" },
			{ offset: 0.15, color: "#1e3a5f" },
			{ offset: 0.4, color: "#1d6c4c" },
			{ offset: 0.7, color: "#38bdf8" },
			{ offset: 1, color: "#4ade80" },
		],
	});

	sciChartSurface.renderableSeries.add(
		new UniformHeatmapRenderableSeries(wasmContext, {
			dataSeries,
			colorMap,
			useLinearTextureFiltering: true,
		}),
	);

	// Source name overlays — pinned to the left edge of each row
	for (let i = 0; i < SOURCES.length; i++) {
		sciChartSurface.annotations.add(
			new TextAnnotation({
				text: SOURCE_LABELS[SOURCES[i]] ?? SOURCES[i],
				x1: 1,
				y1: i,
				xCoordinateMode: ECoordinateMode.DataValue,
				yCoordinateMode: ECoordinateMode.DataValue,
				horizontalAnchorPoint: EHorizontalAnchorPoint.Left,
				verticalAnchorPoint: EVerticalAnchorPoint.Center,
				fontSize: 9,
				textColor: "rgba(226,232,240,0.7)",
				background: "rgba(11,15,20,0.6)",
			}),
		);
	}

	sciChartSurface.background = "transparent";

	let col = 0;

	const push = (values: number[]) => {
		const slot = col % TIME_COLS;
		for (let row = 0; row < nRows; row++) {
			zValues[row][slot] = values[row] ?? 0;
		}
		col++;
		dataSeries.setZValues(zValues);
		sciChartSurface.invalidateElement();
	};

	return {
		sciChartSurface,
		wasmContext,
		controls: { push },
	};
};

type Controls = { push: (values: number[]) => void };

export const SignalHeatmap = memo(function SignalHeatmap() {
	const onInit = useCallback((result: { controls: Controls }) => {
		const tick = () => {
			const snapshot = ConfidenceDataProvider.snapshot();
			const values = SOURCES.map(
				(src) => snapshot.get(src)?.confidence ?? 0,
			);
			result.controls.push(values);
		};

		tick();
		const timer = setInterval(tick, TICK_MS);

		return () => clearInterval(timer);
	}, []);

	return (
		<SciChartReact
			initChart={initSignalHeatmap}
			onInit={onInit}
			className="h-full w-full"
		/>
	);
});
