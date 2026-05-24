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

import type { EnginePulseEvent } from "#/lib/symm/events";
import { eventTimeSec } from "#/lib/symm/events";
import { PulseValueClip, pulseYRange } from "#/lib/symm/chart-range";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const FIFO_CAPACITY = 600;
const PULSE_WINDOW = 48;

export type EnginePulseInitResult = {
	sciChartSurface: SciChartSurface;
	appendPulse: (pulse: EnginePulseEvent) => void;
	dispose: () => void;
};

/** Real-time lines for cross-symbol average prediction and running error. */
class EnginePulseController {
	private surface: SciChartSurface;
	private yAxis: NumericAxis;
	private predictionSeries: XyDataSeries;
	private errorSeries: XyDataSeries;
	private predictionClip = new PulseValueClip();
	private errorClip = new PulseValueClip();
	private pointIndex = 0;
	private frameScheduled = false;

	private constructor(
		surface: SciChartSurface,
		yAxis: NumericAxis,
		predictionSeries: XyDataSeries,
		errorSeries: XyDataSeries,
	) {
		this.surface = surface;
		this.yAxis = yAxis;
		this.predictionSeries = predictionSeries;
		this.errorSeries = errorSeries;
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
			labelPrecision: 4,
		});

		const predictionSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: "Prediction",
			dataIsSortedInX: true,
			containsNaN: false,
			fifoCapacity: FIFO_CAPACITY,
		});
		const errorSeries = new XyDataSeries(wasmContext, {
			dataSeriesName: "Error",
			dataIsSortedInX: true,
			containsNaN: false,
			fifoCapacity: FIFO_CAPACITY,
		});

		sciChartSurface.xAxes.add(xAxis);
		sciChartSurface.yAxes.add(yAxis);
		sciChartSurface.renderableSeries.add(
			new FastLineRenderableSeries(wasmContext, {
				dataSeries: predictionSeries,
				stroke: "#22C55E",
				strokeThickness: 2,
			}),
			new FastLineRenderableSeries(wasmContext, {
				dataSeries: errorSeries,
				stroke: "#F59E0B",
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
			predictionSeries,
			errorSeries,
		);
	}

	get sciChartSurface(): SciChartSurface {
		return this.surface;
	}

	appendPulse(pulse: EnginePulseEvent) {
		const sec = eventTimeSec(pulse);
		this.pointIndex += 1;
		const x = sec + this.pointIndex * 1e-6;

		this.predictionSeries.append(
			x,
			this.predictionClip.clip(pulse.avg_prediction ?? 0),
		);
		this.errorSeries.append(x, this.errorClip.clip(pulse.avg_error ?? 0));
		this.scheduleFrameVisibleRange();
	}

	private scheduleFrameVisibleRange() {
		if (this.frameScheduled) {
			return;
		}

		this.frameScheduled = true;

		requestAnimationFrame(() => {
			this.frameScheduled = false;
			this.applyFrameVisibleRange();
			this.surface.invalidateElement();
		});
	}

	private applyFrameVisibleRange() {
		const range = pulseYRange(
			tailValues(this.predictionSeries, PULSE_WINDOW),
			tailValues(this.errorSeries, PULSE_WINDOW),
			[],
		);

		if (!range) {
			return;
		}

		const current = this.yAxis.visibleRange;

		if (
			current &&
			Math.abs(current.min - range.min) < 1e-6 &&
			Math.abs(current.max - range.max) < 1e-6
		) {
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
		appendPulse: (pulse) => controller.appendPulse(pulse),
		dispose: () => controller.dispose(),
	};
}
