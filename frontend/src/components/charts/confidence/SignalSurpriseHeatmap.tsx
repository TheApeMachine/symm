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
const GAUGE_FULL_SIGMA = 4;

const initSignalSurpriseHeatmap = async (
	rootElement: string | HTMLDivElement,
	sources: readonly string[],
	labels: Record<string, string>,
) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } =
		await SciChartSurface.create(rootElement);
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
					{ offset: 0.2, color: "#312e81" },
					{ offset: 0.45, color: "#7c3aed" },
					{ offset: 0.7, color: "#f97316" },
					{ offset: 1, color: "#fb7185" },
				],
			}),
			useLinearTextureFiltering: true,
		}),
	);

	for (let i = 0; i < sources.length; i += 1) {
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
		for (let row = 0; row < rowCount; row += 1) {
			for (let col = 0; col < TIME_COLS - 1; col += 1) {
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
SignalSurpriseHeatmapBridge feeds live per-signal SNR (temporal surprise) into
the scrolling heatmap at a fixed cadence.
*/
export type SignalSurpriseHeatmapBridge = {
	set: (source: string, snr: number) => void;
	ready: boolean;
};

const SCROLL_MS = 300;

const heatmapValue = (snr: number | undefined): number => {
	if (snr === undefined || snr <= 0) return 0;
	return Math.min(4, (snr / GAUGE_FULL_SIGMA) * 4);
};

export const SignalSurpriseHeatmap = memo(function SignalSurpriseHeatmap({
	sources,
	labels,
	bridgeRef,
}: {
	sources: string[];
	labels: Record<string, string>;
	bridgeRef: RefObject<SignalSurpriseHeatmapBridge>;
}) {
	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) =>
			initSignalSurpriseHeatmap(rootElement, sources, labels),
		[sources, labels],
	);

	const onInit = useCallback(
		(result: { controls: Controls }) => {
			const current = new Map<string, number>(sources.map((s) => [s, 0]));
			const bridge = bridgeRef.current;

			bridge.set = (source, snr) => current.set(source, snr);
			bridge.ready = true;

			const interval = setInterval(() => {
				result.controls.push(sources.map((s) => heatmapValue(current.get(s))));
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
