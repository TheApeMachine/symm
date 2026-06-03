import { memo, useCallback, useMemo } from "react";
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

import { TelemetryManifestPlaceholder } from "#/components/dashboard/TelemetryManifestPlaceholder";
import {
	gaugeLabelFor,
	gaugeSourcesFor,
	hasGaugeSources,
	type LayoutPanel,
} from "#/lib/symm/layout-schema";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";
import { useSymmTelemetryStores } from "#/lib/symm/telemetry-context";

const TIME_COLS = 120;
const GAUGE_FULL_SIGMA = 4;

const initSignalSurpriseHeatmap = async (
	rootElement: string | HTMLDivElement,
	sources: readonly string[],
	panel?: LayoutPanel,
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

	const colorMap = new HeatmapColorMap({
		minimum: 0,
		maximum: 4,
		gradientStops: [
			{ offset: 0, color: "#0b0f14" },
			{ offset: 0.2, color: "#312e81" },
			{ offset: 0.45, color: "#7c3aed" },
			{ offset: 0.7, color: "#f97316" },
			{ offset: 1, color: "#fb7185" },
		],
	});

	sciChartSurface.renderableSeries.add(
		new UniformHeatmapRenderableSeries(wasmContext, {
			dataSeries,
			colorMap,
			useLinearTextureFiltering: true,
		}),
	);

	const labelPanel = panel ?? { type: "gauge_grid" as const };

	for (let index = 0; index < sources.length; index += 1) {
		const source = sources[index];

		sciChartSurface.annotations.add(
			new TextAnnotation({
				text: gaugeLabelFor(labelPanel, source),
				x1: 1,
				y1: index,
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

	return {
		sciChartSurface,
		wasmContext,
		controls: { push },
	};
};

type Controls = { push: (values: number[]) => void };

type SignalSurpriseHeatmapProps = {
	panel?: LayoutPanel;
};

const heatmapValue = (snr: number | undefined): number => {
	if (snr === undefined || snr <= 0) {
		return 0;
	}

	return Math.min(4, (snr / GAUGE_FULL_SIGMA) * 4);
};

/*
SignalSurpriseHeatmap scrolls per-source SNR (temporal standout surprise) so the
main chart area shows which signals are breaking from their own recent baseline.
Confidence heatmap lives with the prediction panel instead.
*/
export const SignalSurpriseHeatmap = memo(function SignalSurpriseHeatmap({
	panel,
}: SignalSurpriseHeatmapProps) {
	const stores = useSymmTelemetryStores();
	const sources = useMemo(() => gaugeSourcesFor(panel), [panel]);

	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) =>
			initSignalSurpriseHeatmap(rootElement, sources, panel),
		[sources, panel],
	);

	const onInit = useCallback(
		(result: { controls: Controls }) => {
			const current = new Map<string, number>();

			for (const source of sources) {
				current.set(source, 0);
			}

			const shiftFrame = () => {
				result.controls.push(
					sources.map((source) => heatmapValue(current.get(source))),
				);
			};

			const unregisters = sources.map((source) =>
				stores.confidence.registerSource(source, (row) => {
					current.set(source, row.snr ?? 0);
					shiftFrame();
				}),
			);

			return () => {
				for (const unregister of unregisters) {
					unregister();
				}
			};
		},
		[sources, stores.confidence],
	);

	if (!hasGaugeSources(panel)) {
		return <TelemetryManifestPlaceholder label="Waiting for gauge manifest…" />;
	}

	return (
		<SciChartReact
			key={sources.join(",")}
			initChart={initChart}
			onInit={onInit}
			className="h-full w-full"
		/>
	);
});
