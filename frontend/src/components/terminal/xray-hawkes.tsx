import { createRef } from "react";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import type { Measurement } from "#/collections/types";
import {
	clearCanvas,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { drawXrayWaiting } from "#/components/terminal/xray-draw";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";

const hawkesCanvasRef = createRef<HTMLCanvasElement>();

type HawkesHistory = {
	buf: number[];
	events: number[];
	mu: number;
	alpha: number;
	beta: number;
	lam: number;
};

const hawkesState: Record<string, HawkesHistory> = {};

const getHawkesState = (symbol: string): HawkesHistory => {
	if (!hawkesState[symbol]) {
		hawkesState[symbol] = {
			buf: [0.2, 0.24, 0.22, 0.31, 0.42, 0.38, 0.55, 0.68, 0.62, 0.48, 0.32, 0.27],
			events: [
				performance.now() - 4200,
				performance.now() - 2500,
				performance.now() - 800,
			],
			mu: 0.2,
			alpha: 0.68,
			beta: 1.25,
			lam: 0.27,
		};
	}

	return hawkesState[symbol]!;
};

/*
paintXrayHawkes draws the focused symbol's arrival process onto the canvas.
It renders the baseline intensity, translucent gold area fill, intensity curve,
and cyan arrival rug ticks along the time axis.
*/

/*
XrayHawkesPanel draws the focused symbol's arrival process.
*/
export const XrayHawkesPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component select="$" registerKey="measurements">
			{({ ref }) => (
				<div
					ref={ref}
					className="relative flex min-h-52.5 flex-1 flex-col border-(--line) border-t"
				>
					<div className="absolute inset-x-0 top-16 bottom-0">
						<canvas
							ref={hawkesCanvasRef}
							data-stream-filter={`source=hawkes,symbol=${focusSymbol}`}
							data-stream-id="at"
							data-stream-time="at"
							data-stream-value="metrics.conditional_intensity:buy.raw"
							data-stream-baseline="metrics.background_rate:buy.raw"
							data-stream-decay="metrics.excitation_decay:buy_from_buy.raw"
							data-stream-window="120"
							data-stream-rug=""
							data-append-limit="512"
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
					<div
						data-scope="source,symbol"
						data-filter={`hawkes,${focusSymbol}`}
						className="pointer-events-none absolute top-3 right-4.5 w-38 text-right font-mono text-[9.5px] text-(--f3) leading-[1.7]"
					>
						<div>
							events{" "}
							<Typography.Span
								data-paint="metrics.event_count.raw"
								data-paint-format=".0f"
								className="text-(--acc)"
							/>
						</div>
						<div>
							λ buy{" "}
							<Typography.Span
								data-paint="metrics.conditional_intensity:buy.raw"
								data-paint-format=".4f"
								data-paint-suffix=" /s"
								className="text-(--f1)"
							/>
						</div>
						<div>
							μ rest{" "}
							<Typography.Span
								data-paint="metrics.background_rate:buy.raw"
								data-paint-format=".4f"
								data-paint-suffix=" /s"
								className="text-(--f1)"
							/>
						</div>
						<div>
							sells{" "}
							<Typography.Span
								data-paint="metrics.event_count:sell.raw"
								data-paint-format=".0f"
								className="text-(--f1)"
							/>
						</div>
						<div className="mt-1 flex items-center justify-end gap-2">
							<span>branching η</span>
							<Typography.Span
								data-paint="metrics.spectral_radius.raw"
								data-paint-format=".3f"
								className="text-(--f1)"
							/>
						</div>
						<div className="mt-1 h-1 overflow-hidden rounded-xs bg-(--line)">
							<div
								data-set="metrics.spectral_radius.raw"
								data-target="style.--eta"
								className="h-full bg-(--acc)"
								style={{ width: "calc(var(--eta, 0) * 100%)" }}
							/>
						</div>
						<div className="mt-0.5 text-[8.5px] text-(--f4)">
							η → 1 · critical cascade
						</div>
					</div>
				</div>
			)}
		</Component>
	);
};
