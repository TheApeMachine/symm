import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { clockAtom, focusAtom, signals } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { type HawkesTraceSample, hawkesTrace } from "./xray-hawkes-trace";

/*
An event's mark determines its excitation jump. Windowed event counts are not
cumulative counters and cannot identify the side of the arriving trade.
A declared zero decay has not been fitted and supplies no intensity curve.
*/
export const hawkesSample = (row: MeasurementT): HawkesTraceSample | null => {
	const metrics = Object.fromEntries(
		row.metrics.map((metric: any) => [String(metric.name), metric.raw]),
	);
	const side = row.provenance.find((value) => value.name === "side")?.value;
	const decay = metrics.excitation_decay;

	if (decay === undefined || decay === 0) return null;

	if (side !== "buy" && side !== "sell")
		throw new Error("Hawkes arrival has no buy/sell mark");

	const intensity = metrics.conditional_intensity;
	const baseline = metrics.background_rate;
	const buyJump = metrics[`excitation_amplitude:buy_from_${side}`];
	const sellJump = metrics[`excitation_amplitude:sell_from_${side}`];

	if (
		[intensity, baseline, buyJump, sellJump].some(
			(value) => value === undefined,
		)
	) {
		throw new Error(
			"Fitted Hawkes arrival is missing its intensity or excitation parameters",
		);
	}

	return {
		at: row.at,
		intensity,
		baseline,
		decay,
		postArrival: intensity + buyJump + sellJump,
	};
};

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		const store = signals.hawkes;
		let samples: HawkesTraceSample[] = [];
		let marketAt = 0n;
		let frame = 0;

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

			if (!canvas || !context) return;

			const ratio = window.devicePixelRatio;
			const width = canvas.clientWidth;
			const height = canvas.clientHeight;
			canvas.width = width * ratio;
			canvas.height = height * ratio;
			context.scale(ratio, ratio);
			const plot = hawkesTrace(samples, width, marketAt);

			if (plot.points.length === 0) return;

			const pad = 14;
			const base = height - 26;
			const top = 30;
			// Keep the scale tied to retained arrivals while a quiet interval scrolls.
			// Rescaling baseline-only pixels would turn rest into a full-height block.
			const peak = Math.max(
				...samples.map((sample) =>
					Math.max(sample.intensity, sample.postArrival),
				),
			);
			const toX = (at: bigint) =>
				pad +
				(Number(at - plot.from) / Number(plot.through - plot.from)) *
					(width - 2 * pad);
			const toY = (value: number) => base - (value / peak) * (base - top);
			const last = samples.at(-1) as HawkesTraceSample;
			set("lambda", `${plot.points.at(-1)?.intensity.toFixed(4)} /s`);
			context.strokeStyle = "#3A342B";
			context.setLineDash([3, 3]);
			context.beginPath();
			context.moveTo(pad, toY(last.baseline));
			context.lineTo(width - pad, toY(last.baseline));
			context.stroke();
			context.setLineDash([]);
			context.beginPath();
			context.moveTo(toX(plot.points[0].at), base);

			for (const point of plot.points)
				context.lineTo(toX(point.at), toY(point.intensity));

			context.lineTo(toX(plot.through), base);
			context.closePath();
			context.fillStyle = "rgba(235, 140, 50, 0.15)";
			context.fill();
			context.beginPath();
			context.moveTo(toX(plot.points[0].at), toY(plot.points[0].intensity));

			for (const point of plot.points)
				context.lineTo(toX(point.at), toY(point.intensity));

			context.strokeStyle = "rgba(235, 140, 50, 0.85)";
			context.lineWidth = 1.6;
			context.stroke();
			context.strokeStyle = "rgba(127, 186, 203, 0.75)";
			context.lineWidth = 1;

			for (const sample of samples) {
				if (sample.at < plot.from) continue;

				const position = toX(sample.at);
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
			const ring = store.state[focusSymbol];
			const rows = Array.from(
				{ length: ring?.getBufferLength() ?? 0 },
				(_, index) => ring.get(index),
			);
			samples = rows.flatMap((row) => {
				const sample = row ? hawkesSample(row) : null;
				return sample ? [sample] : [];
			});
			const latest = rows.at(-1);
			const metrics = Object.fromEntries(
				latest?.metrics.map((metric: any) => [String(metric.name), metric.raw]) ??
					[],
			);
			const fitted = samples.at(-1);
			set("events", metrics.event_count?.toFixed(0) ?? "—");
			set("sells", metrics["event_count:sell"]?.toFixed(0) ?? "—");
			set("mu", fitted ? `${fitted.baseline.toFixed(4)} /s` : "—");
			set("lambda", "—");
			set(
				"eta",
				fitted ? (metrics.branching_spectral_radius?.toFixed(3) ?? "—") : "—",
			);
			const bar = root.current?.querySelector<HTMLElement>("[data-eta-bar]");

			if (bar)
				bar.style.width = fitted
					? `${metrics.branching_spectral_radius * 100}%`
					: "0%";

			if (latest && latest.at > marketAt) marketAt = latest.at;

			schedule();
		};

		update();
		const subscription = store.subscribe(update);
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
			subscription.unsubscribe();
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
