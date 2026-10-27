import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import {
	atoms,
	clockAtom,
	kernelMeasurement,
	kernelSymbolForAggregate,
	signals,
} from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import type { Value } from "#/types/value";
import {
	type HawkesTraceSample,
	hawkesTrace,
	orderTracePoints,
} from "./xray-hawkes-trace";

/*
An event's mark determines its excitation jump. Windowed event counts are not
cumulative counters and cannot identify the side of the arriving trade.
A declared zero decay has not been fitted and supplies no intensity curve.
*/
export const hawkesSample = (row: Value): HawkesTraceSample | null => {
	const metrics = row.metrics();
	const side = row.provenance().side ?? "";

	const buyRate = metrics["arrival_rate:buy"] ?? 0;
	const sellRate = metrics["arrival_rate:sell"] ?? 0;
	const totalArrival = buyRate + sellRate;
	const radius = metrics.branching_spectral_radius ?? 0;

	const legacyEvent =
		side !== "" &&
		metrics["excitation_amplitude:buy_from_buy"] !== undefined;

	if (legacyEvent && metrics.excitation_decay !== undefined) {
		const decay = metrics.excitation_decay;
		if (decay === 0) return null;
		if (side !== "buy" && side !== "sell") {
			throw new Error("Hawkes arrival has no buy/sell mark");
		}
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
			at: row.timestamp,
			intensity,
			baseline,
			decay,
			postArrival: intensity + buyJump + sellJump,
		};
	}

	const level =
		metrics.conditional_intensity !== undefined &&
		metrics.conditional_intensity > 0
			? metrics.conditional_intensity
			: totalArrival;
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
	const fittedDecay = metrics.excitation_decay;
	const decay =
		fittedDecay !== undefined && fittedDecay > 0
			? fittedDecay
			: radius > 0
				? 1.0
				: 0.5;

	return {
		at: row.timestamp,
		intensity: level,
		baseline,
		decay,
		postArrival: level,
	};
};

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		let sample: HawkesTraceSample | undefined;
		let lastFrameTimestamp: bigint | undefined;
		let marketAt = 0n;
		let frame = 0;
		let lastMetrics: Record<string, number> = {};

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

			if (!canvas || !context || !sample) return;

			const ratio = window.devicePixelRatio;
			const width = canvas.clientWidth;
			const height = canvas.clientHeight;
			canvas.width = width * ratio;
			canvas.height = height * ratio;
			context.setTransform(ratio, 0, 0, ratio, 0, 0);
			context.clearRect(0, 0, width, height);

			const HAWKES_VIEWPORT_SPAN = 15_000_000_000n;
			const plot = hawkesTrace(
				[sample],
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
				sample.baseline * 1.12,
				sample.intensity,
				sample.postArrival,
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
			context.moveTo(pad, toY(sample.baseline));
			context.lineTo(width - pad, toY(sample.baseline));
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

			if (sample.at >= plot.from) {
				const position = toX(sample.at);
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
			const aggregate = signals.workspace.state.aggregate;
			const querySymbol = kernelSymbolForAggregate(focusSymbol, aggregate);
			const reading =
				kernelMeasurement("hawkes", querySymbol, aggregate) ??
				kernelMeasurement("hawkes", focusSymbol, aggregate);

			const frameTs = reading?.timestamp ?? aggregate?.timestamp;
			if (reading && frameTs !== undefined) {
				if (
					lastFrameTimestamp === undefined ||
					frameTs > lastFrameTimestamp
				) {
					lastFrameTimestamp = frameTs;
					try {
						const next = hawkesSample(reading);
						if (next) sample = next;
					} catch {
						// Malformed fitted rows are skipped; live metrics stay drawable.
					}
				}
			}

			const live = reading?.metrics() ?? {};
			if (Object.keys(live).length > 0) {
				lastMetrics = live;
			}
			const metrics = Object.keys(live).length > 0 ? live : lastMetrics;
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
				((metrics["background_rate:buy"] ?? 0) +
					(metrics["background_rate:sell"] ?? 0));
			set(
				"mu",
				muRest > 0
					? `${muRest.toFixed(4)} /s`
					: sample
						? `${sample.baseline.toFixed(4)} /s`
						: "—",
			);
			set(
				"lambda",
				metrics.conditional_intensity !== undefined
					? `${metrics.conditional_intensity.toFixed(2)} /s`
					: totalArrival > 0
						? `${totalArrival.toFixed(2)}/s`
						: "—",
			);
			set(
				"eta",
				radius !== undefined
					? radius.toFixed(3)
					: sample
						? (metrics.branching_spectral_radius?.toFixed(3) ?? "—")
						: "—",
			);
			const bar = root.current?.querySelector<HTMLElement>("[data-eta-bar]");

			if (bar) {
				const r = radius ?? metrics.branching_spectral_radius;
				bar.style.width =
					r !== undefined
						? `${Math.min(100, Math.max(0, r * 100)).toFixed(1)}%`
						: "0%";
			}

			if (frameTs !== undefined && frameTs > marketAt) {
				marketAt = frameTs;
			}

			schedule();
		};

		update();
		const subscription = signals.workspace.subscribe(update);
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
