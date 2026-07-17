import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	hawkesCurveFromBuffer,
	hawkesIntensityAt,
} from "#/components/terminal/hawkes-curve";
import { drawXrayWaiting } from "#/components/terminal/xray-draw";
import {
	cascadeLabel,
	formatMetric,
	hawkesMetricsFromBuffer,
} from "#/components/terminal/xray-view";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

/*
XrayHawkesPanel paints fitted Hawkes intensity from measurement epochs.
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
		const segments = hawkesCurveFromBuffer(buffer);
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

		if (segments.length === 0) {
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

		const padX = 18;
		const padTop = 18;
		const padBottom = 28;
		const innerWidth = Math.max(1, width - padX * 2);
		const innerHeight = Math.max(1, height - padTop - padBottom);
		const first = segments[0];
		const latest = segments.at(-1);

		if (first === undefined || latest === undefined) {
			return;
		}

		const fromAt = first.fromAt;
		const throughAt = latest.throughAt;
		const duration = Math.max(1, throughAt - fromAt);
		const maxIntensity = Math.max(
			...segments.flatMap((segment) => [
				segment.beforeArrival,
				segment.afterArrival,
				segment.throughIntensity,
			]),
			latest.baseline,
			1e-9,
		);
		const xFor = (at: number): number =>
			padX + ((at - fromAt) / duration) * innerWidth;
		const yFor = (intensity: number): number =>
			padTop + (1 - intensity / maxIntensity) * innerHeight;
		const trace = (): void => {
			for (const segment of segments) {
				const fromX = xFor(segment.fromAt);
				const throughX = xFor(segment.throughAt);
				context.lineTo(fromX, yFor(segment.beforeArrival));
				context.lineTo(fromX, yFor(segment.afterArrival));

				for (let pixel = Math.floor(fromX) + 1; pixel < throughX; pixel += 1) {
					const at =
						segment.fromAt +
						((pixel - fromX) / Math.max(throughX - fromX, 1)) *
							(segment.throughAt - segment.fromAt);
					context.lineTo(pixel, yFor(hawkesIntensityAt(segment, at)));
				}

				context.lineTo(throughX, yFor(segment.throughIntensity));
			}
		};

		context.fillStyle = "rgba(232, 163, 61, 0.18)";
		context.beginPath();
		context.moveTo(padX, height - padBottom);
		context.lineTo(xFor(first.fromAt), yFor(first.beforeArrival));
		trace();
		context.lineTo(width - padX, height - padBottom);
		context.closePath();
		context.fill();

		context.strokeStyle = "rgba(232, 163, 61, 0.28)";
		context.setLineDash([4, 5]);
		context.beginPath();
		context.moveTo(padX, yFor(latest.baseline));
		context.lineTo(width - padX, yFor(latest.baseline));
		context.stroke();
		context.setLineDash([]);

		context.strokeStyle = TERMINAL_COLORS.amber;
		context.lineWidth = 1.8;
		context.beginPath();
		context.moveTo(xFor(first.fromAt), yFor(first.beforeArrival));
		trace();
		context.stroke();

		const x = xFor(latest.throughAt);
		const y = yFor(latest.throughIntensity);

		context.fillStyle = TERMINAL_COLORS.amber;
		context.shadowBlur = 10;
		context.shadowColor = TERMINAL_COLORS.amber;
		context.beginPath();
		context.arc(x, y, 3.5, 0, Math.PI * 2);
		context.fill();
		context.shadowBlur = 0;
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "10px JetBrains Mono, monospace";
		context.fillText(`λ ${latest.throughIntensity.toFixed(2)}`, 18, height - 9);
	};

	useDirectStorePaint(paint, [measurementsStore, appStore], []);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const observer = new ResizeObserver(paint);
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
				<canvas
					ref={canvasRef}
					className="absolute inset-0 block size-full"
				/>
			</div>
		</div>
	);
};
