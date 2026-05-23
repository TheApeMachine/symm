import {
	DateTimeNumericAxis,
	EAutoRange,
	ENumericFormat,
	FastLineRenderableSeries,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	SciChartSurface,
	XyDataSeries,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";

import type { FieldSnapshotEvent } from "#/lib/symm/events";
import { eventTimeSec } from "#/lib/symm/events";
import { PulseValueClip, pulseYRange } from "#/lib/symm/chart-range";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const FIFO_CAPACITY = 600;
const PULSE_WINDOW = 48;

export type EnginePulseInitResult = {
	sciChartSurface: SciChartSurface;
	appendField: (snapshot: FieldSnapshotEvent) => void;
	dispose: () => void;
};

/** Real-time lines for aggregate fluid scalars (Re, turb, div) with outlier clipping. */
class EnginePulseController {
	private surface: SciChartSurface;
	private yAxis: NumericAxis;
	private reSeries: XyDataSeries;
	private turbSeries: XyDataSeries;
	private divSeries: XyDataSeries;
	private reClip = new PulseValueClip();
	private turbClip = new PulseValueClip();
	private divClip = new PulseValueClip();
	private pointIndex = 0;

	private constructor(
		surface: SciChartSurface,
		yAxis: NumericAxis,
		reSeries: XyDataSeries,
		turbSeries: XyDataSeries,
		divSeries: XyDataSeries,
	) {
		this.surface = surface;
		this.yAxis = yAxis;
		this.reSeries = reSeries;
		this.turbSeries = turbSeries;
		this.divSeries = divSeries;
	}

	static async create(
		rootElement: HTMLDivElement,
	): Promise<EnginePulseController> {
		await ensureSciChartWasm();
		const { sciChartSurface, wasmContext } =
			await SciChartSurface.create(rootElement);
		sciChartSurface.title = "";

		const xAxis = new DateTimeNumericAxis(wasmContext, {
			autoRange: EAutoRange.Always,
			growBy: new NumberRange(0.02, 0.04),
		});
		const yAxis = new NumericAxis(wasmContext, {
			autoRange: EAutoRange.Never,
			growBy: new NumberRange(0.08, 0.12),
			labelFormat: ENumericFormat.Decimal,
			labelPrecision: 3,
		});

		const reSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: "Re",
			fifoCapacity: FIFO_CAPACITY,
		});
		const turbSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: "Turb",
			fifoCapacity: FIFO_CAPACITY,
		});
		const divSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: "Div",
			fifoCapacity: FIFO_CAPACITY,
		});

		sciChartSurface.xAxes.add(xAxis);
		sciChartSurface.yAxes.add(yAxis);
		sciChartSurface.renderableSeries.add(
			new FastLineRenderableSeries(wasmContext, {
				dataSeries: reSeries,
				stroke: "#22C55E",
				strokeThickness: 2,
			}),
			new FastLineRenderableSeries(wasmContext, {
				dataSeries: turbSeries,
				stroke: "#F59E0B",
				strokeThickness: 2,
			}),
			new FastLineRenderableSeries(wasmContext, {
				dataSeries: divSeries,
				stroke: "#3B82F6",
				strokeThickness: 2,
			}),
		);
		sciChartSurface.chartModifiers.add(
			new ZoomPanModifier(),
			new MouseWheelZoomModifier(),
			new ZoomExtentsModifier(),
		);

		return new EnginePulseController(
			sciChartSurface,
			yAxis,
			reSeries,
			turbSeries,
			divSeries,
		);
	}

	get sciChartSurface(): SciChartSurface {
		return this.surface;
	}

	appendField(snapshot: FieldSnapshotEvent) {
		const field = snapshot.field;
		if (!field) {
			return;
		}

		const sec = eventTimeSec(snapshot);
		this.pointIndex += 1;
		const x = sec + this.pointIndex * 1e-6;

		this.reSeries.append(x, this.reClip.clip(field.re));
		this.turbSeries.append(x, this.turbClip.clip(field.turb));
		this.divSeries.append(x, this.divClip.clip(field.div));
		this.frameVisibleRange();
		this.surface.invalidateElement();
	}

	private frameVisibleRange() {
		const range = pulseYRange(
			tailValues(this.reSeries, PULSE_WINDOW),
			tailValues(this.turbSeries, PULSE_WINDOW),
			tailValues(this.divSeries, PULSE_WINDOW),
		);

		if (!range) {
			return;
		}

		this.yAxis.visibleRange = new NumberRange(range.min, range.max);
	}

	dispose() {
		// SciChartReact owns surface teardown.
	}
}

function tailValues(series: XyDataSeries, count: number): number[] {
	const total = series.count();
	if (total <= 0) {
		return [];
	}

	const start = Math.max(0, total - count);
	const values = series.getNativeYValues();
	const tail: number[] = [];

	for (let index = start; index < total; index++) {
		tail.push(values.get(index));
	}

	return tail;
}

export async function initEnginePulseChart(
	rootElement: string | HTMLDivElement,
): Promise<EnginePulseInitResult> {
	if (typeof rootElement === "string") {
		throw new Error("initEnginePulseChart requires an HTMLDivElement root");
	}

	const controller = await EnginePulseController.create(rootElement);

	return {
		sciChartSurface: controller.sciChartSurface,
		appendField: (snapshot) => controller.appendField(snapshot),
		dispose: () => controller.dispose(),
	};
}
