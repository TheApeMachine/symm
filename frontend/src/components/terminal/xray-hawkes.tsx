import { useEffect, useRef } from "react";
import { useSelector } from "@tanstack/react-store";
import { focusStore, getMeasurementStore } from "#/collections/app";
import type { FrameBuffer } from "#/collections/app";
import type { Measurement } from "#/providers/telemetry/telemetry/measurement";
import {
	getRetainedHawkes,
	retainHawkesMetric,
} from "#/components/terminal/xray-view";
import { Typography } from "#/components/ui/typography";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		const hawkesStore = getMeasurementStore("hawkes");

		const updateFromState = (state: FrameBuffer<Measurement>) => {
			if (!root.current) return;
			const row = state.getLast();

			if (row) {
				for (let j = 0; j < row.metricsLength(); j++) {
					const m = row.metrics(j, metricObj);
					if (m) {
						retainHawkesMetric(focusSymbol, m.name() ?? "", m.raw());
					}
				}
			}

			const retained = getRetainedHawkes(focusSymbol);

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

			const lambda =
				retained["conditional_intensity:buy"] ??
				retained.conditional_intensity ??
				retained["arrival_rate:buy"] ??
				retained.arrival_rate;
			if (typeof lambda === "number") {
				set("lambda", `${lambda.toFixed(4)} /s`);
			}

			const mu = retained["background_rate:buy"] ?? retained.background_rate;
			if (typeof mu === "number") {
				set("mu", `${mu.toFixed(4)} /s`);
			}

			const sells =
				retained["event_count:sell"] ??
				retained["conditional_intensity:sell"] ??
				retained["arrival_rate:sell"];
			if (typeof sells === "number") {
				set("sells", sells.toFixed(0));
			}

			const eta =
				retained.branching_spectral_radius ??
				retained.spectral_radius ??
				retained.branching;
			if (typeof eta === "number") {
				set("eta", eta.toFixed(3));
				const etaBar = root.current.querySelector<HTMLElement>("[data-eta-bar]");
				if (etaBar instanceof HTMLElement) {
					etaBar.style.width = `calc(${Math.min(1, Math.max(0, eta))} * 100%)`;
				}
			}

			const canvas = hawkesCanvasRef.current;
			if (canvas && state.getSize() > 1) {
				const ctx = canvas.getContext("2d");
				if (ctx) {
					const dpr = window.devicePixelRatio || 1;
					const w = canvas.clientWidth * dpr;
					const h = canvas.clientHeight * dpr;
					canvas.width = w;
					canvas.height = h;
					ctx.clearRect(0, 0, w, h);

					const count = state.getSize();
					const intensities: number[] = [];
					for (let i = 0; i < count; i++) {
						const r = state.get(i);
						if (!r) continue;
						let intensityVal = 0;
						for (let j = 0; j < r.metricsLength(); j++) {
							const m = r.metrics(j, metricObj);
							if (m) {
								const name = m.name();
								if (
									name === "conditional_intensity:buy" ||
									name === "conditional_intensity" ||
									name === "arrival_rate:buy" ||
									name === "arrival_rate"
								) {
									intensityVal = m.raw();
									break;
								}
							}
						}
						intensities.push(intensityVal);
					}

					const observedMax = Math.max(0, ...intensities);
					const maxL = observedMax > 0 ? observedMax : 1;
					const pad = 14 * (window.devicePixelRatio || 1);
					const base = h - 22 * (window.devicePixelRatio || 1);
					const topMargin = 20 * (window.devicePixelRatio || 1);
					const toX = (idx: number) =>
						pad + (idx / Math.max(1, intensities.length - 1)) * (w - pad * 2);
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

					if (intensities.length > 1) {
						ctx.beginPath();
						ctx.moveTo(toX(0), base);
						for (let i = 0; i < intensities.length; i++) {
							ctx.lineTo(toX(i), toY(intensities[i] ?? 0));
						}
						ctx.lineTo(toX(intensities.length - 1), base);
						ctx.closePath();
						ctx.fillStyle = "rgba(235, 140, 50, 0.15)";
						ctx.fill();

						ctx.strokeStyle = "rgba(235, 140, 50, 0.85)";
						ctx.lineWidth = 1.6;
						ctx.beginPath();
						for (let i = 0; i < intensities.length; i++) {
							const x = toX(i);
							const y = toY(intensities[i] ?? 0);
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
	}, [focusSymbol]);

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
					λ buy <Typography.Span data-f="lambda" className="text-(--f1)" />
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
