import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";

/*
XrayHawkesPanel draws the focused symbol's buy-side arrival process.

Every Hawkes measurement is stamped at a real trade arrival rather than on a
clock, so this is an event stream, not a series sampled at a fixed rate. The
canvas is therefore laid out on arrival time, ticks the arrivals it was built
from, and relaxes λ toward μ at the fitted β between them — the kernel's own
shape. Drawing straight lines between arrivals would assert a linear decay the
fit explicitly denies, and spacing them evenly would erase the inter-arrival
timing, which in a self-exciting process is the whole signal.

The branching ratio comes from the fitted spectral radius, so the bar states how
close the cascade sits to criticality without labelling a regime the kernel
never claimed.
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
					{/*
						The plot is inset below the caption so its own axis labels are not
						painted underneath it. A canvas carries an intrinsic aspect ratio,
						so the inset lives on a plain box and the canvas fills it.
					*/}
					<div className="absolute inset-x-0 top-16 bottom-0">
						<canvas
							data-stream-filter={`source=hawkes,symbol=${focusSymbol}`}
							data-stream-id="at"
							data-stream-time="at"
							data-stream-value="metrics.conditional_intensity:buy.raw"
							data-stream-baseline="metrics.baseline_intensity:buy.raw"
							data-stream-decay="metrics.decay_rate.raw"
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
							λ(t) = μ + Σ α·e^(-β(t-tᵢ)) · ticks mark observed arrivals
						</div>
					</div>
					<div
						data-scope="source,symbol"
						data-filter={`hawkes,${focusSymbol}`}
						className="pointer-events-none absolute top-3 right-4.5 w-38 text-right font-mono text-[9.5px] text-(--f3) leading-[1.7]"
					>
						<div>
							λ buy{" "}
							<Typography.Span
								data-paint="metrics.conditional_intensity:buy.raw"
								data-paint-format=".4f"
								data-paint-suffix=" /s"
								className="text-(--acc)"
							/>
						</div>
						<div>
							μ rest{" "}
							<Typography.Span
								data-paint="metrics.baseline_intensity:buy.raw"
								data-paint-format=".4f"
								data-paint-suffix=" /s"
								className="text-(--f1)"
							/>
						</div>
						<div>
							events{" "}
							<Typography.Span
								data-paint="metrics.event_count.raw"
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
