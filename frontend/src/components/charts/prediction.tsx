import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { PredictiveCodingCanvas } from "#/components/charts/prediction-canvas";
import { Component } from "#/components/ui/component";

/*
The resonance batch carries every settled carrier, so the row is pinned by
symbol. Reading index 0 was the same thing while the solver published a single
row; it now names whichever carrier sorts first, which is a different symbol
from one frame to the next.
*/
const ScalarDiagnostics = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="resonance">
			{({ ref }) => (
				<div
					ref={ref}
					data-scope="symbol"
					data-filter={focusSymbol}
					className="grid grid-cols-5 gap-px overflow-hidden border border-(--line) bg-(--line)"
				>
					<div className="bg-[#0a0907] px-2 py-1.5">
						<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							confidence
						</div>
						<div
							data-paint="forecast.confidence"
							data-paint-format=".1%"
							className="mt-0.5 font-mono text-[11px] text-(--up)"
						>
							—
						</div>
					</div>
					<div className="bg-[#0a0907] px-2 py-1.5">
						<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							horizon
						</div>
						<div
							data-paint="forecast.supportedHorizon"
							data-paint-format=".0f"
							data-paint-suffix=" ticks"
							className="mt-0.5 font-mono text-[11px] text-(--f2)"
						>
							—
						</div>
					</div>
					<div className="bg-[#0a0907] px-2 py-1.5">
						<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							alpha
						</div>
						<div
							data-paint="alpha"
							data-paint-format=".4f"
							className="mt-0.5 font-mono text-[11px] text-(--acc)"
						>
							—
						</div>
					</div>
					<div className="min-w-0 bg-[#0a0907] px-2 py-1.5">
						<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							surprise
						</div>
						<div
							data-paint="surprise"
							data-paint-format=".2f"
							className="mt-0.5 truncate font-mono text-[11px] text-(--warning)"
						>
							—
						</div>
					</div>
					<div className="min-w-0 bg-[#0a0907] px-2 py-1.5">
						<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							energy
						</div>
						<div
							data-paint="energy"
							data-paint-format=".2f"
							className="mt-0.5 truncate font-mono text-[11px] text-(--info)"
						>
							—
						</div>
					</div>
				</div>
			)}
		</Component>
	);
};

/*
TerminalPredictionChart is the settled predictive-coding readout.

The scalars are painted straight from the focused resonance row. Beneath them
the hierarchy is a chart, not a row of bars: it needs a lane per layer, each on
its own scale, and it needs history — a reconstruction error only means
something against the errors before it. One bar per vector element was neither,
which is why the latent read as a flat band and the two-point forward curve as a
solid block.
*/
export const TerminalPredictionChart = () => (
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-7">
		<ScalarDiagnostics />
		<PredictiveCodingCanvas className="block min-h-0 w-full flex-1" />
	</div>
);
