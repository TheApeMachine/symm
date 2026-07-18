import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
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
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

/*
XrayHawkesPanel paints a dense Hawkes intensity path: arrival impulses and the
exponential decay between them, matching the terminal mockup sampler.
*/
export const XrayHawkesPanel = () => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const branchingRef = useRef<HTMLSpanElement>(null);
	const intensityRef = useRef<HTMLSpanElement>(null);
	const cascadeRef = useRef<HTMLDivElement>(null);

	const paint = () => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const context = resizeCanvas(canvas);

		if (context === null) {
			return;
		}

		const symbol = appStore.state.focusSymbol;
		const buffer = measurementsStore.state.measurements[symbol]?.hawkes;
		const metrics = hawkesMetricsFromBuffer(buffer);
		const cascade = cascadeLabel(metrics.branching);
		const series = hawkesSeriesFromBuffer(buffer);
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
		const maxIntensity =
			Math.max(series.baseline * 1.2, ...series.samples) * 1.1;
		const xFor = (index: number): number =>
			padX + (index / (series.samples.length - 1)) * innerWidth;
		const yFor = (intensity: number): number =>
			base - (intensity / maxIntensity) * (base - padTop);
		const latest = series.samples[series.samples.length - 1] ?? series.baseline;

		context.strokeStyle = "rgba(232, 163, 61, 0.28)";
		context.setLineDash([3, 3]);
		context.lineWidth = 1;
		context.beginPath();
		context.moveTo(padX, yFor(series.baseline));
		context.lineTo(width - padX, yFor(series.baseline));
		context.stroke();
		context.setLineDash([]);

		context.beginPath();
		context.moveTo(xFor(0), base);

		for (let index = 0; index < series.samples.length; index += 1) {
			context.lineTo(xFor(index), yFor(series.samples[index] ?? 0));
		}

		context.lineTo(xFor(series.samples.length - 1), base);
		context.closePath();
		context.fillStyle = "rgba(232, 163, 61, 0.14)";
		context.fill();

		context.strokeStyle = TERMINAL_COLORS.amber;
		context.lineWidth = 1.6;
		context.beginPath();

		for (let index = 0; index < series.samples.length; index += 1) {
			const x = xFor(index);
			const y = yFor(series.samples[index] ?? 0);

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
			xFor(series.samples.length - 1),
			yFor(latest),
			3.5,
			0,
			Math.PI * 2,
		);
		context.fill();
		context.shadowBlur = 0;
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "10px JetBrains Mono, monospace";
		context.fillText(`λ ${latest.toFixed(2)}`, 18, height - 9);
	};

	const paintRef = useRef(paint);
	useDirectStorePaint(paint, [measurementsStore, appStore], []);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const observer = new ResizeObserver(() => {
			paintRef.current();
		});
		observer.observe(canvas);

		return () => observer.disconnect();
	}, []);

	return (
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
				<canvas ref={canvasRef} className="absolute inset-0 block size-full" />
			</div>
		</div>
	);
};
