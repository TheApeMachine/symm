import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { atoms, clockAtom, signals } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import { kernelRowAt } from "#/lib/kernel-row";
import {
	type HawkesTraceSample,
	hawkesTrace,
	orderTracePoints,
} from "./xray-hawkes-trace";

const flatHawkesMetrics = (
	row: Record<string, unknown> | undefined,
): Record<string, number> => {
	if (!row || typeof row !== "object") return {};
	return Object.fromEntries(
		Object.entries(row).filter(
			(entry): entry is [string, number] =>
				typeof entry[1] === "number" && Number.isFinite(entry[1]),
		),
	);
};

const traceFromMetrics = (
	metrics: Record<string, number>,
	at: bigint,
): HawkesTraceSample | undefined => {
	const buyRate = metrics["arrival_rate:buy"] ?? 0;
	const sellRate = metrics["arrival_rate:sell"] ?? 0;
	const totalArrival = buyRate + sellRate;
	const level =
		metrics.conditional_intensity !== undefined &&
		metrics.conditional_intensity > 0
			? metrics.conditional_intensity
			: totalArrival;
	if (level <= 0) return undefined;

	const muRest =
		metrics.background_rate ??
		(metrics["background_rate:buy"] ?? 0) +
			(metrics["background_rate:sell"] ?? 0);
	const baseline =
		muRest > 0 && muRest < level
			? muRest
			: totalArrival > 0 && totalArrival < level
				? totalArrival
				: level * 0.85;
	const radius = metrics.branching_spectral_radius ?? 0;
	const fittedDecay = metrics.excitation_decay;
	const decay =
		fittedDecay !== undefined && fittedDecay > 0
			? fittedDecay
			: radius > 0
				? 1
				: 0.5;

	return { at, intensity: level, baseline, decay, postArrival: level };
};

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		let marketAt = 0n;
		let frame = 0;
		let traceSample: HawkesTraceSample | undefined;

		const set = (field: string, value: string) => {
			const element = root.current?.querySelector<HTMLElement>(
				`[data-f=${field}]`,
			);
			if (element) element.textContent = value;
		};

		const paint = () => {
			frame = 0;
			const canvas = hawkesCanvasRef.current;
			const context = canvas?.getContext("2d");
			if (!canvas || !context || !traceSample) return;

			const ratio = window.devicePixelRatio;
			const width = canvas.clientWidth;
			const height = canvas.clientHeight;
			canvas.width = width * ratio;
			canvas.height = height * ratio;
			context.setTransform(ratio, 0, 0, ratio, 0, 0);
			context.clearRect(0, 0, width, height);

			const HAWKES_VIEWPORT_SPAN = 15_000_000_000n;
			const plot = hawkesTrace(
				[traceSample],
				width,
				marketAt,
				HAWKES_VIEWPORT_SPAN,
			);
			const curve = orderTracePoints(plot.points);
			if (curve.length === 0) return;

			const pad = 14;
			const base = height - 26;
			const top = 30;
			const peak = Math.max(
				traceSample.baseline * 1.12,
				traceSample.intensity,
				traceSample.postArrival,
				...curve.map((p) => p.intensity),
				1e-6,
			);
			const toX = (at: bigint) =>
				pad +
				(Number(at - plot.from) / Number(plot.through - plot.from)) *
					(width - 2 * pad);
			const toY = (value: number) => base - (value / peak) * (base - top);

			set("lambda", `${curve.at(-1)?.intensity.toFixed(4)} /s`);
			context.strokeStyle = "#3A342B";
			context.setLineDash([3, 3]);
			context.beginPath();
			context.moveTo(pad, toY(traceSample.baseline));
			context.lineTo(width - pad, toY(traceSample.baseline));
			context.stroke();
			context.setLineDash([]);
			context.beginPath();
			context.moveTo(toX(plot.from), base);
			for (const point of curve) {
				context.lineTo(toX(point.at), toY(point.intensity));
			}
			context.lineTo(toX(plot.through), base);
			context.closePath();
			context.fillStyle = "rgba(235, 140, 50, 0.15)";
			context.fill();
			context.beginPath();
			context.moveTo(toX(curve[0].at), toY(curve[0].intensity));
			for (const point of curve) {
				context.lineTo(toX(point.at), toY(point.intensity));
			}
			context.strokeStyle = "rgba(235, 140, 50, 0.85)";
			context.lineWidth = 1.6;
			context.stroke();

			if (traceSample.at >= plot.from) {
				const position = toX(traceSample.at);
				context.strokeStyle = "rgba(127, 186, 203, 0.75)";
				context.lineWidth = 1;
				context.beginPath();
				context.moveTo(position, base);
				context.lineTo(position, base + 8);
				context.stroke();
			}

			context.fillStyle = "#8b8578";
			context.font = "9px monospace";
			context.fillText(
				`${(Number(plot.through - plot.from) / 1e9).toFixed(1)}s · market time`,
				pad,
				height - 3,
			);
		};

		const schedule = () => {
			if (!frame) frame = requestAnimationFrame(paint);
		};

		const update = () => {
			const hit = kernelRowAt(
				signals.hawkes.state,
				focusSymbol,
			) as Record<string, unknown> | undefined;
			const metrics = flatHawkesMetrics(hit);
			const buyRate = metrics["arrival_rate:buy"];
			const sellRate = metrics["arrival_rate:sell"];
			const radius = metrics.branching_spectral_radius;
			const totalArrival = (buyRate ?? 0) + (sellRate ?? 0);

			set(
				"events",
				metrics.event_count !== undefined
					? metrics.event_count.toFixed(0)
					: buyRate !== undefined
						? `${buyRate.toFixed(2)}/s`
						: "—",
			);
			set(
				"sells",
				metrics["event_count:sell"] !== undefined
					? metrics["event_count:sell"].toFixed(0)
					: sellRate !== undefined
						? `${sellRate.toFixed(2)}/s`
						: "—",
			);
			const muRest =
				metrics.background_rate ??
				(metrics["background_rate:buy"] ?? 0) +
					(metrics["background_rate:sell"] ?? 0);
			set("mu", muRest > 0 ? `${muRest.toFixed(4)} /s` : "—");
			set(
				"lambda",
				metrics.conditional_intensity !== undefined
					? `${metrics.conditional_intensity.toFixed(2)} /s`
					: totalArrival > 0
						? `${totalArrival.toFixed(2)}/s`
						: "—",
			);
			set("eta", radius !== undefined ? radius.toFixed(3) : "—");

			const bar = root.current?.querySelector<HTMLElement>("[data-eta-bar]");
			if (bar) {
				bar.style.width =
					radius !== undefined
						? `${Math.min(100, Math.max(0, radius * 100)).toFixed(1)}%`
						: "0%";
			}

			if (marketAt > 0n && Object.keys(metrics).length > 0) {
				traceSample = traceFromMetrics(metrics, marketAt);
			}

			schedule();
		};

		update();
		const hawkesSub = signals.hawkes.subscribe(update);
		const unsubscribeClock = clockAtom.subscribe((timestamp) => {
			if (timestamp === null) return;
			const at = BigInt(Math.trunc(timestamp * 1e6));
			if (at <= marketAt) return;
			marketAt = at;
			schedule();
		});
		const resize = new ResizeObserver(schedule);
		if (hawkesCanvasRef.current) resize.observe(hawkesCanvasRef.current);

		return () => {
			hawkesSub.unsubscribe();
			unsubscribeClock.unsubscribe();
			resize.disconnect();
			cancelAnimationFrame(frame);
		};
	}, [focusSymbol]);

	return (
		<div
			ref={root}
			className="relative flex min-h-52.5 flex-1 flex-col border-(--line) border-t"
		>
			<div className="absolute inset-0">
				<canvas
					ref={hawkesCanvasRef}
					className="absolute inset-0 block size-full"
				/>
			</div>
			<div className="pointer-events-none absolute top-3 left-4.5">
				<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
					Hawkes self-exciting intensity
				</div>
				<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
					arrivals observed · λ(t) = μ + Σ α·e^(-β(t-tᵢ)) once fitted
				</div>
			</div>
			<div className="pointer-events-none absolute top-3 right-4.5 w-38 text-right font-mono text-[9.5px] text-(--f3) leading-[1.7]">
				<div>
					events <Typography.Span data-f="events" className="text-(--acc)" />
				</div>
				<div>
					λ(t) <Typography.Span data-f="lambda" className="text-(--f1)" />
				</div>
				<div>
					μ rest <Typography.Span data-f="mu" className="text-(--f1)" />
				</div>
				<div>
					sells <Typography.Span data-f="sells" className="text-(--f1)" />
				</div>
				<div className="mt-1 flex items-center justify-end gap-2">
					<span>branching η</span>
					<Typography.Span data-f="eta" className="text-(--f1)" />
				</div>
				<div className="mt-1 h-1 overflow-hidden rounded-xs bg-(--line)">
					<div
						data-eta-bar
						className="h-full bg-(--acc)"
						style={{ width: "0%" }}
					/>
				</div>
				<div className="mt-0.5 text-[8.5px] text-(--f4)">
					η → 1 · critical cascade
				</div>
			</div>
		</div>
	);
};