import { useEffect, useRef } from "react";
import type { FrameBuffer } from "#/collections/app";
import { getMeasurementStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import type { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import { EnvelopeMeasurementMetric } from "#/providers/telemetry/telemetry/envelope-measurement-metric";
import { EnvelopeMetric } from "#/providers/telemetry/telemetry/envelope-metric";

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

/*
rowIntensity reads this row's own conditional_intensity — the fitted total
λ(t) — falling back to arrival_rate only when conditional_intensity was not
reported on this exact row (the model is still cold). The two are distinct,
non-interchangeable statistics (fitted intensity vs. a cumulative empirical
rate), so this never mixes a conditional_intensity from one row with an
arrival_rate from another; each returned sample is one row's own reading.
*/
const rowIntensity = (row: EnvelopeMeasurement): number | null => {
	let conditionalIntensity: number | null = null;
	let arrivalRate: number | null = null;

	for (let j = 0; j < row.metricsLength(); j++) {
		const m = row.metrics(j, metricObj);
		const value = m?.value(valueObj);
		if (!m || !value) continue;

		if (m.key() === "conditional_intensity") conditionalIntensity = value.raw();
		if (m.key() === "arrival_rate") arrivalRate = value.raw();
	}

	return conditionalIntensity ?? arrivalRate;
};

export const XrayHawkesPanel = () => {
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		const hawkesStore = getMeasurementStore("hawkes");

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
			// sell-side excitation together — and arrival_rate is a distinct,
			// unrelated empirical statistic (a cumulative average rate, not
			// the fitted intensity). They must not stand in for each other:
			// only fall back to arrival_rate when conditional_intensity has
			// genuinely never been reported yet (the model is still cold).
			const lambda = retained.conditional_intensity ?? retained.arrival_rate;
			if (typeof lambda === "number") {
				set("lambda", `${lambda.toFixed(4)} /s`);
			}

			const mu = retained.background_rate;
			if (typeof mu === "number") {
				set("mu", `${mu.toFixed(4)} /s`);
			}

			const beta = retained["excitation_decay:buy_from_buy"];

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
					const intensityRows: Array<{ at: bigint; raw: number }> = [];
					for (let i = 0; i < count; i++) {
						const r = state.get(i);
						if (!r) continue;

						const intensity = rowIntensity(r);
						if (intensity !== null) {
							intensityRows.push({ at: r.atNs(), raw: intensity });
						}
					}
					// intensitySeriesFromRingRows collapses to a plain number[] (its
					// tested contract), but the decay curve below needs each
					// sample's own epoch too, so the same by-epoch dedup is redone
					// here to pair each value back up with its timestamp.
					const byEpoch = new Map<bigint, number>();
					for (const row of intensityRows) {
						byEpoch.set(row.at, row.raw);
					}
					const samples = [...byEpoch.entries()]
						.sort((left, right) =>
							left[0] < right[0] ? -1 : left[0] > right[0] ? 1 : 0,
						)
						.map(([at, raw]) => ({ at, raw }));

					// Real intensity ranges from a few hundredths (quiet symbols)
					// to several tens (a fresh burst), so the vertical scale must
					// follow the series actually observed rather than a fixed
					// ceiling: a constant tuned for one regime silently flattens
					// every symbol whose scale sits far from it. The bottom is
					// still floored (as the reference keeps a resting floor so the
					// μ baseline and quiet symbols stay readable) — otherwise a
					// ring window with only resting samples inflates the tallest
					// spike of every quieter symbol far past its own scale.
					const observedMax = Math.max(0, ...samples.map((s) => s.raw));
					const muFloor = typeof mu === "number" && mu > 0 ? mu : 0;
					const maxL = Math.max(observedMax, muFloor, 1.2, Number.EPSILON) * 1.1;
					const pad = 14 * (window.devicePixelRatio || 1);
					const base = h - 22 * (window.devicePixelRatio || 1);
					const topMargin = 20 * (window.devicePixelRatio || 1);

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
							at > tMin
								? Number(at - tMin) / Number(tMax - tMin)
								: 0;
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

					// A Hawkes intensity jumps instantly at an arrival and decays
					// exponentially toward μ afterward — it never ramps up smoothly
					// into an arrival. Each inter-sample gap therefore renders as a
					// vertical jump at the arrival's own epoch followed by the
					// exponential decay μ + (λ_prev − μ)·e^(−β·Δt) across the real
					// gap. That keeps every point clocked to true time instead of
					// evenly spacing a resampled array. The next arrival only lands
					// (as its own vertical jump) at its own epoch. Empty gaps (bad
					// epoch ordering) are skipped.
					if (samples.length > 1) {
						const decayRate = typeof beta === "number" && beta > 0 ? beta : 1;
						const restingRate = muFloor;
						const STEPS_PER_GAP = 24;

						const plot: Array<[number, number]> = [[toX(samples[0].at), toY(samples[0].raw)]];

						for (let i = 1; i < samples.length; i++) {
							const prev = samples[i - 1];
							const next = samples[i];
							if (!prev || !next) continue;

							if (next.at <= prev.at) continue;

							const gapSeconds = Number(next.at - prev.at) / 1e9;

							// Decay inward from the arrival's intensity, keeping each
							// point on its own real time coordinate.
							for (let step = 1; step <= STEPS_PER_GAP; step++) {
								const seconds = gapSeconds * (step / STEPS_PER_GAP);
								const epoch = prev.at + BigInt(Math.round(seconds * 1e9));
								const value =
									step === STEPS_PER_GAP
										? next.raw
										: restingRate +
											(prev.raw - restingRate) * Math.exp(-decayRate * seconds);
								plot.push([toX(epoch), toY(value)]);
							}
						}

						ctx.beginPath();
						ctx.moveTo(pad, base);
						for (const [x, y] of plot) {
							ctx.lineTo(x, y);
						}
						ctx.lineTo(plot.length ? plot[plot.length - 1][0] : pad, base);
						ctx.closePath();
						ctx.fillStyle = "rgba(235, 140, 50, 0.15)";
						ctx.fill();

						ctx.strokeStyle = "rgba(235, 140, 50, 0.85)";
						ctx.lineWidth = 1.6;
						ctx.beginPath();
						for (let i = 0; i < plot.length; i++) {
							const [x, y] = plot[i];
							if (i === 0) ctx.moveTo(x, y);
							else ctx.lineTo(x, y);
						}
						ctx.stroke();
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
	}, []);

	return (
		<div
			ref={root}
			className="relative flex min-h-52.5 flex-1 flex-col border-(--line) border-t"
		>
			<div className="absolute inset-x-0 top-16 bottom-0">
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
