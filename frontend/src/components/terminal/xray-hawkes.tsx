import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import type { FrameBuffer } from "#/collections/app";
import { focusStore, getMeasurementStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import type { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import { EnvelopeMeasurementMetric } from "#/providers/telemetry/telemetry/envelope-measurement-metric";
import { EnvelopeMetric } from "#/providers/telemetry/telemetry/envelope-metric";
import {
	type HawkesTracePoint,
	type HawkesTraceSample,
	hawkesTrace,
} from "./xray-hawkes-trace";

const metricObj = new EnvelopeMeasurementMetric();
const valueObj = new EnvelopeMetric();

/*
latestMetrics holds the last known value of every metric name seen anywhere
in the store's ring, scanning back from the newest row. Each Hawkes emission
carries only a handful of the ~30 named bindings (the rest are 0 elsewhere
in this same commit, sharing one Frame) — reading only the latest row drops
metrics like background_rate/excitation_decay whenever the newest tick didn't
happen to report them, even though a recent tick did. This holds the last
real value per key instead of treating that absence as "no data."
*/
const latestMetrics = (
	state: FrameBuffer<EnvelopeMeasurement>,
): Record<string, number> => {
	const values: Record<string, number> = {};
	const count = state.getBufferLength();

	for (let i = count - 1; i >= 0; i--) {
		const row = state.get(i);
		if (!row) continue;

		for (let j = 0; j < row.metricsLength(); j++) {
			const m = row.metrics(j, metricObj);
			const value = m?.value(valueObj);
			const key = m?.key() ?? "";
			if (m && value && !(key in values)) {
				values[key] = value.raw();
			}
		}
	}

	return values;
};

type HawkesObservation = {
	at: bigint;
	intensity: number;
	baseline: number;
	decay: number;
	buyCount: number;
	sellCount: number;
	buyJump: number;
	sellJump: number;
};

/*
rowObservation reads one fitted Hawkes observation and the exact model
parameters needed to render the event that follows its pre-arrival intensity.
An empirical arrival rate is not a substitute for a fitted λ(t).
*/
const rowObservation = (row: EnvelopeMeasurement): HawkesObservation | null => {
	const metrics: Record<string, number> = {};

	for (let j = 0; j < row.metricsLength(); j++) {
		const m = row.metrics(j, metricObj);
		const value = m?.value(valueObj);
		if (!m || !value) continue;

		metrics[m.key() ?? ""] = value.raw();
	}

	const required = [
		"conditional_intensity",
		"background_rate",
		"excitation_decay:buy_from_buy",
		"event_count:buy",
		"event_count:sell",
		"excitation_amplitude:buy_from_buy",
		"excitation_amplitude:sell_from_buy",
		"excitation_amplitude:buy_from_sell",
		"excitation_amplitude:sell_from_sell",
	];

	if (required.some((key) => metrics[key] === undefined)) {
		return null;
	}

	return {
		at: row.atNs(),
		intensity: metrics.conditional_intensity as number,
		baseline: metrics.background_rate as number,
		decay: metrics["excitation_decay:buy_from_buy"] as number,
		buyCount: metrics["event_count:buy"] as number,
		sellCount: metrics["event_count:sell"] as number,
		buyJump:
			(metrics["excitation_amplitude:buy_from_buy"] as number) +
			(metrics["excitation_amplitude:sell_from_buy"] as number),
		sellJump:
			(metrics["excitation_amplitude:buy_from_sell"] as number) +
			(metrics["excitation_amplitude:sell_from_sell"] as number),
	};
};

const traceSamples = (observations: HawkesObservation[]): HawkesTraceSample[] =>
	observations.map((observation, index) => {
		const previous = observations[index - 1];
		let jump = 0;

		if (
			previous &&
			observation.buyCount > previous.buyCount &&
			observation.sellCount === previous.sellCount
		) {
			jump = observation.buyJump;
		}

		if (
			previous &&
			observation.sellCount > previous.sellCount &&
			observation.buyCount === previous.buyCount
		) {
			jump = observation.sellJump;
		}

		return {
			at: observation.at,
			intensity: observation.intensity,
			postArrival: observation.intensity + jump,
			baseline: observation.baseline,
			decay: observation.decay,
		};
	});

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		const hawkesStore = getMeasurementStore("hawkes", focusSymbol);

		const updateFromState = (state: FrameBuffer<EnvelopeMeasurement>) => {
			if (!root.current) return;
			const retained = latestMetrics(state);

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
				if (el) el.textContent = value;
			};

			// Reset readouts, the eta bar, and the canvas before applying
			// retained metrics so sparse data cannot preserve stale geometry.
			for (const field of ["events", "lambda", "mu", "sells", "eta"]) {
				set(field, "");
			}

			const etaBar = root.current.querySelector<HTMLElement>("[data-eta-bar]");
			if (etaBar instanceof HTMLElement) {
				etaBar.style.width = "0%";
			}

			const resetCanvas = hawkesCanvasRef.current;
			if (resetCanvas) {
				const resetCtx = resetCanvas.getContext("2d");
				if (resetCtx) {
					resetCtx.clearRect(0, 0, resetCanvas.width, resetCanvas.height);
				}
			}

			const events = retained.event_count ?? retained["event_count:buy"];
			if (typeof events === "number") {
				set("events", events.toFixed(0));
			}

			// conditional_intensity is the fitted total λ(t) — buy-side plus
			// sell-side excitation together. The empirical arrival rate is a
			// distinct statistic and must not stand in for the fitted intensity.
			const lambda = retained.conditional_intensity;
			if (typeof lambda === "number") {
				set("lambda", `${lambda.toFixed(4)} /s`);
			}

			const mu = retained.background_rate;
			if (typeof mu === "number") {
				set("mu", `${mu.toFixed(4)} /s`);
			}

			const sells = retained["event_count:sell"];
			if (typeof sells === "number") {
				set("sells", sells.toFixed(0));
			}

			const eta =
				retained.branching_spectral_radius ??
				retained.spectral_radius ??
				retained.branching;
			if (typeof eta === "number") {
				set("eta", eta.toFixed(3));
				const etaBar =
					root.current.querySelector<HTMLElement>("[data-eta-bar]");
				if (etaBar instanceof HTMLElement) {
					etaBar.style.width = `calc(${Math.min(1, Math.max(0, eta))} * 100%)`;
				}
			}

			const canvas = hawkesCanvasRef.current;
			if (canvas && state.getBufferLength() > 1) {
				const ctx = canvas.getContext("2d");
				if (ctx) {
					const dpr = window.devicePixelRatio || 1;
					const w = canvas.clientWidth * dpr;
					const h = canvas.clientHeight * dpr;
					canvas.width = w;
					canvas.height = h;
					ctx.clearRect(0, 0, w, h);

					const count = state.getBufferLength();
					const observations: HawkesObservation[] = [];
					for (let i = 0; i < count; i++) {
						const r = state.get(i);
						if (!r) continue;

						const observation = rowObservation(r);
						if (observation !== null) {
							observations.push(observation);
						}
					}

					const byEpoch = new Map<bigint, HawkesObservation>();
					for (const observation of observations) {
						byEpoch.set(observation.at, observation);
					}
					const samples = traceSamples(
						[...byEpoch.values()].sort((left, right) =>
							left.at < right.at ? -1 : left.at > right.at ? 1 : 0,
						),
					);
					const plot = hawkesTrace(samples, canvas.clientWidth);

					// Scale the actual visible intensity readings. Holding an old
					// ceiling after its spike left the ring flattened the next
					// regime into the floor and made the chart look empty.
					const observedMax = Math.max(
						0,
						...plot.map((point) => point.intensity),
					);
					const muFloor = typeof mu === "number" && mu > 0 ? mu : 0;
					const maxL = Math.max(observedMax, muFloor, Number.EPSILON) * 1.1;
					const pad = 14 * (window.devicePixelRatio || 1);
					const base = h - 22 * (window.devicePixelRatio || 1);
					const topMargin = 30 * (window.devicePixelRatio || 1);

					// The x-axis spans the real epochs of the visible samples so
					// each spike sits at the moment it was observed and decay over a
					// long gap stretches over that same gap on screen. Mapping x to
					// a resampled step index (rather than true elapsed time) lets
					// spikes get compressed/stretched as the ring window slides and
					// real inter-arrival gaps vary — the source of the constantly
					// shifting spike shapes.
					const tMin = samples.length ? samples[0].at : 0n;
					const tLast = samples.length ? samples[samples.length - 1].at : 0n;
					const tMax = tLast > tMin ? tLast : tMin + 1n;
					const toX = (at: bigint) => {
						const fraction =
							at > tMin ? Number(at - tMin) / Number(tMax - tMin) : 0;
						return pad + Math.max(0, Math.min(1, fraction)) * (w - pad * 2);
					};
					const toY = (val: number) => base - (val / maxL) * (base - topMargin);

					if (typeof mu === "number" && mu > 0) {
						ctx.strokeStyle = "#3A342B";
						ctx.setLineDash([3, 3]);
						ctx.lineWidth = 1;
						ctx.beginPath();
						ctx.moveTo(pad, toY(mu));
						ctx.lineTo(w - pad, toY(mu));
						ctx.stroke();
						ctx.setLineDash([]);
					}

					// Each interval uses the model published with its own event.
					// Applying the newest μ and β to the entire history mixed fits
					// and produced the isolated full-height needles in the broken
					// chart.
					if (samples.length > 1) {
						ctx.beginPath();
						ctx.moveTo(pad, base);
						for (const point of plot) {
							ctx.lineTo(toX(point.at), toY(point.intensity));
						}
						ctx.lineTo(toX(tLast), base);
						ctx.closePath();
						ctx.fillStyle = "rgba(235, 140, 50, 0.15)";
						ctx.fill();

						ctx.strokeStyle = "rgba(235, 140, 50, 0.85)";
						ctx.lineWidth = 1.6;
						ctx.beginPath();
						for (let i = 0; i < plot.length; i++) {
							const point = plot[i] as HawkesTracePoint;
							const x = toX(point.at);
							const y = toY(point.intensity);
							if (i === 0) ctx.moveTo(x, y);
							else ctx.lineTo(x, y);
						}
						ctx.stroke();

						ctx.strokeStyle = "rgba(127, 186, 203, 0.75)";
						ctx.lineWidth = window.devicePixelRatio || 1;

						for (const sample of samples) {
							const x = toX(sample.at);
							ctx.beginPath();
							ctx.moveTo(x, base);
							ctx.lineTo(x, base + 8 * (window.devicePixelRatio || 1));
							ctx.stroke();
						}
					}
				}
			}
		};

		updateFromState(hawkesStore.state);
		const subscription = hawkesStore.subscribe((state) => {
			updateFromState(state);
		});

		return () => {
			subscription.unsubscribe();
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
