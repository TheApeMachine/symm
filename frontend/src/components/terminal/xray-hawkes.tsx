import { createRef } from "react";
import type { Measurement, MeasurementEpoch } from "#/collections/types";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { hawkesSeriesFromBuffer } from "#/components/terminal/hawkes-curve";
import { drawXrayWaiting } from "#/components/terminal/xray-draw";
import {
	cascadeLabel,
	formatMetric,
	hawkesMetricsFromBuffer,
} from "#/components/terminal/xray-view";
import { frameRows } from "#/providers/frame-history";

const hawkesCanvasRef = createRef<HTMLCanvasElement>();
const branchingRef = createRef<HTMLSpanElement>();
const intensityRef = createRef<HTMLSpanElement>();
const cascadeRef = createRef<HTMLDivElement>();

/*
paintXrayHawkes reconstructs Hawkes intensity from retained measurement epochs
and draws the resulting temporal curve and current metric readouts.
*/
export const paintXrayHawkes = (value: unknown, focusSymbol: string) => {
	const canvas = hawkesCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	const measurements = frameRows<Measurement>(value);
	const frames = measurements.filter(
		(measurement) =>
			measurement.source === "hawkes" &&
			(focusSymbol === "" || measurement.symbol === focusSymbol),
	);
	const byAt = new Map<string, Measurement[]>();

	for (const row of frames) {
		const group = byAt.get(row.at) ?? [];
		group.push(row);
		byAt.set(row.at, group);
	}

	const epochs: MeasurementEpoch[] = [...byAt.values()].map((readings) => ({
		at: readings[0]?.at ?? "",
		readings,
		publishedAt: readings.at(-1)?.at ?? readings[0]?.at ?? "",
	}));
	const metrics = hawkesMetricsFromBuffer(frames);
	const cascade = cascadeLabel(metrics.branching);
	const series = hawkesSeriesFromBuffer(epochs);
	const width = canvas.clientWidth;
	const height = canvas.clientHeight;

	if (branchingRef.current !== null) {
		branchingRef.current.textContent = formatMetric(metrics.branching);
		branchingRef.current.style.color = cascade.color;
	}

	if (intensityRef.current !== null) {
		intensityRef.current.textContent = formatMetric(metrics.intensity, 2);
	}

	if (cascadeRef.current !== null) {
		cascadeRef.current.textContent = `cascade ${cascade.label}`;
		cascadeRef.current.style.color = cascade.color;
	}

	if (series === null || series.samples.length < 2) {
		drawXrayWaiting(
			context,
			width,
			height,
			"waiting for fitted hawkes intensity",
		);
		return;
	}

	clearCanvas(context, width, height);
	drawGrid(context, width, height, 18);

	const padX = 14;
	const padTop = 30;
	const padBottom = 26;
	const innerWidth = Math.max(1, width - padX * 2);
	const base = height - padBottom;
	// Draw (λ − μ) / μ so spike height is excitation in baseline units,
	// not a window-max auto-scale that reflows every paint.
	const unit = series.baseline > 0 ? series.baseline : 1;
	const normalized = series.samples.map(
		(sample) => Math.max(0, sample - series.baseline) / unit,
	);
	const framePeak = Math.max(series.peakExcess / unit, ...normalized, 0);
	const yMax = Math.max(1, framePeak) * 1.15;
	const xFor = (index: number): number =>
		padX + (index / (series.samples.length - 1)) * innerWidth;
	const yFor = (value: number): number =>
		base - (value / yMax) * (base - padTop);
	const latest = series.samples[series.samples.length - 1] ?? series.baseline;
	const latestExcess = Math.max(0, latest - series.baseline);
	const latestNorm = normalized[normalized.length - 1] ?? 0;

	context.beginPath();
	context.moveTo(xFor(0), base);

	for (let index = 0; index < normalized.length; index += 1) {
		context.lineTo(xFor(index), yFor(normalized[index] ?? 0));
	}

	context.lineTo(xFor(normalized.length - 1), base);
	context.closePath();
	context.fillStyle = "rgba(232, 163, 61, 0.14)";
	context.fill();

	context.strokeStyle = TERMINAL_COLORS.amber;
	context.lineWidth = 1.6;
	context.beginPath();

	for (let index = 0; index < normalized.length; index += 1) {
		const x = xFor(index);
		const y = yFor(normalized[index] ?? 0);

		if (index === 0) {
			context.moveTo(x, y);
			continue;
		}

		context.lineTo(x, y);
	}

	context.stroke();

	const duration = Math.max(1, series.throughAt - series.fromAt);
	context.strokeStyle = "#7FBACB";
	context.lineWidth = 1;

	for (const eventAt of series.events) {
		const age = series.throughAt - eventAt;

		if (age < 0 || age > duration) {
			continue;
		}

		const x = padX + ((eventAt - series.fromAt) / duration) * innerWidth;
		context.globalAlpha = Math.max(0.2, 1 - age / duration);
		context.beginPath();
		context.moveTo(x, base);
		context.lineTo(x, base + 8);
		context.stroke();
	}

	context.globalAlpha = 1;
	context.fillStyle = TERMINAL_COLORS.amber;
	context.shadowBlur = 10;
	context.shadowColor = TERMINAL_COLORS.amber;
	context.beginPath();
	context.arc(
		xFor(normalized.length - 1),
		yFor(latestNorm),
		3.5,
		0,
		Math.PI * 2,
	);
	context.fill();
	context.shadowBlur = 0;
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "10px JetBrains Mono, monospace";
	context.fillText(
		`λ−μ ${latestExcess.toFixed(2)}  (λ−μ)/μ ${latestNorm.toFixed(2)}  λ ${latest.toFixed(2)}  μ ${series.baseline.toFixed(2)}`,
		18,
		height - 9,
	);
};

/*
XrayHawkesPanel is the static shell for Hawkes intensity. DRAW paints via
paintXrayHawkes.
*/
export const XrayHawkesPanel = () => (
	<div className="flex min-h-[210px] flex-1 flex-col border-(--line) border-t">
		<div className="flex items-start justify-between gap-3 px-[18px] pt-3 pb-2">
			<div>
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Hawkes self-exciting intensity
				</div>
				<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
					λ(t) = μ + Σ α·e^(-β(t-tᵢ)) · order-flow arrivals
				</div>
			</div>
			<div className="shrink-0 text-right font-mono text-[10px]">
				<div>
					<span className="text-(--f3)">η = α/β = </span>
					<span ref={branchingRef} />
				</div>
				<div>
					<span className="text-(--f3)">λ now </span>
					<span ref={intensityRef} className="text-(--acc)" />
				</div>
				<div ref={cascadeRef} />
			</div>
		</div>
		<div className="relative min-h-0 flex-1">
			<canvas
				ref={hawkesCanvasRef}
				className="absolute inset-0 block size-full"
			/>
		</div>
	</div>
);
