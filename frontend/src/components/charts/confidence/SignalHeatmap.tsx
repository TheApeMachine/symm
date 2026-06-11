import { memo } from "react";
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
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { ensureSciChartWasm } from "#/lib/utils";

const TIME_COLS = 120;

const initSignalHeatmap = async (
	rootElement: string | HTMLDivElement,
	sources: readonly string[],
	labels: Record<string, string>,
) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{ freezeWhenOutOfView: true },
	);
	const rowCount = sources.length;
	const zValues = zeroArray2D([rowCount, TIME_COLS]);

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
			visibleRange: new NumberRange(-0.5, rowCount - 0.5),
		}),
	);

	const dataSeries = new UniformHeatmapDataSeries(wasmContext, {
		zValues,
		xStart: 0,
		xStep: 1,
		yStart: 0,
		yStep: 1,
	});

	sciChartSurface.renderableSeries.add(
		new UniformHeatmapRenderableSeries(wasmContext, {
			dataSeries,
			colorMap: new HeatmapColorMap({
				minimum: 0,
				maximum: 4,
				gradientStops: [
					{ offset: 0, color: "#0b0f14" },
					{ offset: 0.15, color: "#1e3a5f" },
					{ offset: 0.4, color: "#1d6c4c" },
					{ offset: 0.7, color: "#38bdf8" },
					{ offset: 1, color: "#4ade80" },
				],
			}),
			useLinearTextureFiltering: true,
		}),
	);

	for (let rowIndex = 0; rowIndex < sources.length; rowIndex += 1) {
		sciChartSurface.annotations.add(
			new TextAnnotation({
				text: labels[sources[rowIndex]] ?? sources[rowIndex],
				x1: 1,
				y1: rowIndex,
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

	const sourceIndex = new Map(sources.map((source, index) => [source, index]));

	const addData = (frame: Record<string, unknown>) => {
		const source = frame.source;

		if (typeof source !== "string") {
			return;
		}

		const confidence = frame.confidence;

		if (typeof confidence !== "number" || !Number.isFinite(confidence)) {
			return;
		}

		const rowIndex = sourceIndex.get(source);

		if (rowIndex === undefined) {
			return;
		}

		const row = zValues[rowIndex];
		const value = Math.min(4, Math.max(0, confidence) * 4);

		for (let col = 0; col < TIME_COLS - 1; col += 1) {
			row[col] = row[col + 1];
		}

		row[TIME_COLS - 1] = value;
		dataSeries.setZValues(zValues);
		sciChartSurface.invalidateElement();
	};

	return { sciChartSurface, wasmContext, addData };
};

export const SignalHeatmap = memo(function SignalHeatmap({
	sources,
	labels,
}: {
	sources: string[];
	labels: Record<string, string>;
}) {
	return (
		<SciChartReact
			key={sources.join(",")}
			initChart={(rootElement) =>
				initSignalHeatmap(rootElement, sources, labels)
			}
			onInit={(result) => {
				appStore.actions.updateConfidenceHeatmapUpdater(result.addData);

				return () => appStore.actions.updateConfidenceHeatmapUpdater(null);
			}}
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
		/>
	);
});
