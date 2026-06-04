import { type RefObject, memo, useCallback } from "react";
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
import { ensureSciChartWasm } from "#/lib/utils";

const TIME_COLS = 120;

const initSignalHeatmap = async (
	rootElement: string | HTMLDivElement,
	sources: readonly string[],
	labels: Record<string, string>,
) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } =
		await SciChartSurface.create(rootElement);
	const nRows = sources.length;
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

	for (let i = 0; i < sources.length; i++) {
		sciChartSurface.annotations.add(
			new TextAnnotation({
				text: labels[sources[i]] ?? sources[i],
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

	const push = (values: number[]) => {
		for (let row = 0; row < nRows; row++) {
			for (let col = 0; col < TIME_COLS - 1; col++) {
				zValues[row][col] = zValues[row][col + 1];
			}
			zValues[row][TIME_COLS - 1] = values[row] ?? 0;
		}
		dataSeries.setZValues(zValues);
		sciChartSurface.invalidateElement();
	};

	return { sciChartSurface, wasmContext, controls: { push } };
};

type Controls = { push: (values: number[]) => void };

/*
SignalHeatmapBridge feeds live per-signal values into the scrolling heatmap. The
chart owns a fixed-cadence scroll so the time axis stays even regardless of how
often signals tick.
*/
export type SignalHeatmapBridge = {
	set: (source: string, value: number) => void;
	ready: boolean;
};

const SCROLL_MS = 300;

const heatmapValue = (confidence: number): number =>
	Math.min(4, Math.max(0, confidence) * 4);

export const SignalHeatmap = memo(function SignalHeatmap({
	sources,
	labels,
	bridgeRef,
}: {
	sources: string[];
	labels: Record<string, string>;
	bridgeRef: RefObject<SignalHeatmapBridge>;
}) {
	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) =>
			initSignalHeatmap(rootElement, sources, labels),
		[sources, labels],
	);

	const onInit = useCallback(
		(result: { controls: Controls }) => {
			const current = new Map<string, number>(sources.map((s) => [s, 0]));
			const bridge = bridgeRef.current;

			bridge.set = (source, value) => current.set(source, value);
			bridge.ready = true;

			const interval = setInterval(() => {
				result.controls.push(sources.map((s) => heatmapValue(current.get(s) ?? 0)));
			}, SCROLL_MS);

			return () => {
				clearInterval(interval);
				bridge.set = () => {};
				bridge.ready = false;
			};
		},
		[sources, bridgeRef],
	);

	return (
		<SciChartReact
			key={sources.join(",")}
			initChart={initChart}
			onInit={onInit}
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
		/>
	);
});
