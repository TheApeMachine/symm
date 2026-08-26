import { useRef } from "react";
import { useSelector } from "@tanstack/react-store";
import { focusStore, measurementStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);
	const hawkesCanvasRef = useRef<HTMLCanvasElement>(null);

	measurementStore.subscribe((state) => {
		if (!root.current) return;
		const ring = state.hawkes?.[focusSymbol];
		const row = ring?.getLast();

		/*
			Backend updates arrive sparsely: a row may carry only a subset of the
			vocabulary. Writing a default zero for an absent metric would blank a
			live reading, so each readout is left at its current value unless the
			row actually carries the metric.
		*/
		if (!row) return;

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
			if (el) el.textContent = value;
		};

		const metricsMap: Record<string, { raw: number; normalized: number }> = {};
		for (let j = 0; j < row.metricsLength(); j++) {
			const m = row.metrics(j, metricObj);
			if (m) {
				metricsMap[m.name() ?? ""] = {
					raw: m.raw(),
					normalized: m.normalized(),
				};
			}
		}

		const write = (
			metric: string,
			q: string,
			format: (raw: number) => string,
		) => {
			const m = metricsMap[metric];
			if (m !== undefined) {
				set(q, format(m.raw));
			}
		};

		write("event_count", "events", (raw) => raw.toFixed(0));
		write(
			"conditional_intensity:buy",
			"lambda",
			(raw) => `${raw.toFixed(4)} /s`,
		);
		write("background_rate:buy", "mu", (raw) => `${raw.toFixed(4)} /s`);
		write("event_count:sell", "sells", (raw) => raw.toFixed(0));
		write("spectral_radius", "eta", (raw) => raw.toFixed(3));

		const etaBar = root.current.querySelector<HTMLElement>("[data-eta-bar]");
		const eta = metricsMap.spectral_radius?.raw;

		if (etaBar instanceof HTMLElement && typeof eta === "number") {
			etaBar.style.width = `calc(${Math.min(1, Math.max(0, eta))} * 100%)`;
		}

		const canvas = hawkesCanvasRef.current;
		if (canvas && ring && ring.getSize() > 1) {
			const ctx = canvas.getContext("2d");
			if (ctx) {
				const w = (canvas.width =
					canvas.clientWidth * (window.devicePixelRatio || 1));
				const h = (canvas.height =
					canvas.clientHeight * (window.devicePixelRatio || 1));
				ctx.clearRect(0, 0, w, h);
				ctx.strokeStyle = "rgba(235, 140, 50, 0.7)";
				ctx.lineWidth = 1.5;
				ctx.beginPath();

				const count = ring.getSize();
				for (let i = 0; i < count; i++) {
					const r = ring.get(i);
					if (!r) continue;
					let intensity = 0;
					for (let j = 0; j < r.metricsLength(); j++) {
						const m = r.metrics(j, metricObj);
						if (m && m.name() === "conditional_intensity:buy") {
							intensity = m.raw();
							break;
						}
					}
					const x = (i / Math.max(1, count - 1)) * w;
					const y =
						h - Math.min(1, Math.max(0, intensity / 10)) * (h - 20) - 10;
					if (i === 0) ctx.moveTo(x, y);
					else ctx.lineTo(x, y);
				}
				ctx.stroke();
			}
		}
	});

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
