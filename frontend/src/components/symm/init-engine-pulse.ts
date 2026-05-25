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
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const FIFO_CAPACITY = 600;
const PULSE_WINDOW = 48;
const PULSE_CLIP_CAP = 64;

const clipPulseValue = (value: number, history: number[]): number => {
	if (!Number.isFinite(value)) {
		return 0;
	}

	if (history.length < 8) {
		history.push(value);

		if (history.length > PULSE_CLIP_CAP) {
			history.shift();
		}

		return value;
	}

	const sorted = [...history].sort((left, right) => left - right);
	const median = sorted[Math.floor(sorted.length / 2)];
	const deviations = sorted
		.map((sample) => Math.abs(sample - median))
		.sort((left, right) => left - right);
	const mad = deviations[Math.floor(deviations.length / 2)];
	const spread = Math.max(mad * 3, Math.abs(median) * 0.5, 1e-6);
	const fence = Math.abs(median) + spread;
	const clipped = Math.max(-fence, Math.min(fence, value));

	history.push(clipped);

	if (history.length > PULSE_CLIP_CAP) {
		history.shift();
	}

	return clipped;
};

const pulseYRange = (
	reValues: number[],
	turbValues: number[],
	divValues: number[],
): { min: number; max: number } | null => {
	const values = [...reValues, ...turbValues, ...divValues].filter((value) =>
		Number.isFinite(value),
	);

	if (values.length === 0) {
		return { min: -0.01, max: 0.01 };
	}

	const min = Math.min(...values);
	const max = Math.max(...values);

	if (min === max) {
		const pad = Math.max(Math.abs(min) * 0.1, 0.01);
		return { min: min - pad, max: max + pad };
	}

	const span = max - min;
	const pad = span * 0.12;

	return { min: min - pad, max: max + pad };
};

export const initEnginePulseChart = async (rootElement: HTMLDivElement) => {
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

	const predictionHistory: number[] = [];
	const errorHistory: number[] = [];
	let pointIndex = 0;
	let frameScheduled = false;

	const tailValues = (series: XyDataSeries, count: number): number[] => {
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
	};

	const applyFrameVisibleRange = () => {
		const range = pulseYRange(
			tailValues(predictionSeries, PULSE_WINDOW),
			tailValues(errorSeries, PULSE_WINDOW),
			[],
		);

		if (!range) {
			return;
		}

		const current = yAxis.visibleRange;

		if (
			current &&
			Math.abs(current.min - range.min) < 1e-6 &&
			Math.abs(current.max - range.max) < 1e-6
		) {
			return;
		}

		yAxis.visibleRange = new NumberRange(range.min, range.max);
	};

	const scheduleFrameVisibleRange = () => {
		if (frameScheduled) {
			return;
		}

		frameScheduled = true;

		requestAnimationFrame(() => {
			frameScheduled = false;
			applyFrameVisibleRange();
			sciChartSurface.invalidateElement();
		});
	};

	const appendPulse = (pulse: EnginePulseEvent) => {
		const sec = eventTimeSec(pulse);
		pointIndex += 1;
		const x = sec + pointIndex * 1e-6;

		predictionSeries.append(
			x,
			clipPulseValue(pulse.avg_prediction ?? 0, predictionHistory),
		);
		errorSeries.append(
			x,
			clipPulseValue(pulse.avg_error ?? 0, errorHistory),
		);
		scheduleFrameVisibleRange();
	};

	return {
		sciChartSurface,
		appendPulse,
	};
};
