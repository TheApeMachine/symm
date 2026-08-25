import { createRef, useEffect } from "react";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { registerStreamCanvas, unregisterStreamCanvas } from "#/providers/stream-canvas";
import { Typography } from "#/components/ui/typography";
import { measurementsStore, useSubscribe } from "#/providers/ws-stores";

const hawkesCanvasRef = createRef<HTMLCanvasElement>();

const streamDataset = (focusSymbol: string) => ({
	streamFilter: `source=hawkes,symbol=${focusSymbol}`,
	streamId: "at",
	streamTime: "at",
	streamValue: "metrics.conditional_intensity:buy.raw",
	streamBaseline: "metrics.background_rate:buy.raw",
	streamDecay: "metrics.excitation_decay:buy_from_buy.raw",
	streamWindow: "120",
	streamRug: "",
	appendLimit: "512",
});

export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(measurementsStore, (state) => {
		const row = state.measurements[`hawkes\u0000${focusSymbol}`]?.latest();

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("events", (row?.metrics?.event_count?.raw ?? 0).toFixed(0));
		set("lambda", `${(row?.metrics?.["conditional_intensity:buy"]?.raw ?? 0).toFixed(4)} /s`);
		set("mu", `${(row?.metrics?.["background_rate:buy"]?.raw ?? 0).toFixed(4)} /s`);
		set("sells", (row?.metrics?.["event_count:sell"]?.raw ?? 0).toFixed(0));
		set("eta", (row?.metrics?.spectral_radius?.raw ?? 0).toFixed(3));

		const etaBar = root.current?.querySelector<HTMLElement>("[data-eta-bar]");
		const eta = row?.metrics?.spectral_radius?.raw;

		if (etaBar instanceof HTMLElement && typeof eta === "number") {
			etaBar.style.width = `calc(${Math.min(1, Math.max(0, eta))} * 100%)`;
		}
	}, [focusSymbol]);

	useEffect(() => {
		const canvas = hawkesCanvasRef.current;

		if (canvas === null) {
			return;
		}

		registerStreamCanvas(canvas, streamDataset(focusSymbol));

		return () => unregisterStreamCanvas(canvas);
	}, [focusSymbol]);

	return (
		<div ref={root} className="relative flex min-h-52.5 flex-1 flex-col border-(--line) border-t">
			<div className="absolute inset-x-0 top-16 bottom-0">
				<canvas ref={hawkesCanvasRef} className="absolute inset-0 block size-full" />
			</div>
			<div className="pointer-events-none absolute top-3 left-4.5">
				<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">Hawkes self-exciting intensity</div>
				<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">arrivals observed · λ(t) = μ + Σ α·e^(-β(t-tᵢ)) once fitted</div>
			</div>
			<div className="pointer-events-none absolute top-3 right-4.5 w-38 text-right font-mono text-[9.5px] text-(--f3) leading-[1.7]">
				<div>events <Typography.Span data-f="events" className="text-(--acc)" /></div>
				<div>λ buy <Typography.Span data-f="lambda" className="text-(--f1)" /></div>
				<div>μ rest <Typography.Span data-f="mu" className="text-(--f1)" /></div>
				<div>sells <Typography.Span data-f="sells" className="text-(--f1)" /></div>
				<div className="mt-1 flex items-center justify-end gap-2">
					<span>branching η</span>
					<Typography.Span data-f="eta" className="text-(--f1)" />
				</div>
				<div className="mt-1 h-1 overflow-hidden rounded-xs bg-(--line)">
					<div data-eta-bar className="h-full bg-(--acc)" style={{ width: "0%" }} />
				</div>
				<div className="mt-0.5 text-[8.5px] text-(--f4)">η → 1 · critical cascade</div>
			</div>
		</div>
	);
};
