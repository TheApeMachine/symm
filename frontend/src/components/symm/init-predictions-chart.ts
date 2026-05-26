import {
	SciChartSurface,
	NumericAxis,
	EAxisAlignment,
	NumberRange,
	FastLineRenderableSeries,
	XyDataSeries,
	AUTO_COLOR,
	ZoomExtentsModifier,
	MouseWheelZoomModifier,
	ZoomPanModifier,
	RolloverModifier,
} from "scichart";

import { GridLayoutModifier } from "#/components/symm/layout-modifier";
import {
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	type SignalSource,
} from "#/lib/symm/signal-confidence";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const FIFO_CAPACITY = 600;

export type PredictionsChartControls = {
	appendReading: (source: SignalSource, x: number, value: number) => void;
};

export const drawExample = async (rootElement: string | HTMLDivElement) => {
	await ensureSciChartWasm();

	const { wasmContext, sciChartSurface } =
		await SciChartSurface.create(rootElement);

	sciChartSurface.xAxes.add(new NumericAxis(wasmContext));
	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			axisAlignment: EAxisAlignment.Left,
			growBy: new NumberRange(0.05, 0.05),
		}),
	);

	const seriesBySource = new Map<SignalSource, XyDataSeries>();

	for (const source of SIGNAL_SOURCES) {
		const dataSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: SIGNAL_LABELS[source],
			dataIsSortedInX: true,
			containsNaN: false,
			fifoCapacity: FIFO_CAPACITY,
		});

		sciChartSurface.renderableSeries.add(
			new FastLineRenderableSeries(wasmContext, {
				dataSeries,
				stroke: AUTO_COLOR,
				strokeThickness: 3,
			}),
		);
		seriesBySource.set(source, dataSeries);
	}

	sciChartSurface.chartModifiers.add(
		new ZoomExtentsModifier({ modifierGroup: "chart" }),
		new MouseWheelZoomModifier({ modifierGroup: "chart" }),
		new ZoomPanModifier({ modifierGroup: "chart" }),
		new RolloverModifier({ modifierGroup: "chart" }),
	);

	const gridLayout = new GridLayoutModifier();
	sciChartSurface.chartModifiers.add(gridLayout);

	sciChartSurface.zoomExtents();

	const appendReading = (source: SignalSource, x: number, value: number) => {
		const dataSeries = seriesBySource.get(source);

		if (!dataSeries) {
			return;
		}

		dataSeries.append(x, value);
		sciChartSurface.invalidateElement();
	};

	const setIsGridLayoutMode = (value: boolean) => {
		gridLayout.isGrid = value;
	};

	return {
		wasmContext,
		sciChartSurface,
		setIsGridLayoutMode,
		controls: { appendReading },
	};
};
