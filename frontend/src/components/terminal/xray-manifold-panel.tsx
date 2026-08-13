import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

/*
XrayManifoldPanel is the manifold reading.

The predictive network publishes its settled state one carrier at a time, so the
panel scopes to the focused symbol's frame rather than whichever frame happens
to sit first in the batch. Relative residual precision, task skill, horizon,
and reach are shown with the same names the learning package exposes.
*/
export const XrayManifoldPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="resonance">
			{({ ref, className }) => (
				<div
					ref={ref}
					className={cn(
						"flex flex-col gap-2 border-(--line) border-t px-3.5 py-3",
						className,
					)}
				>
					<div>
						<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Manifold reading
						</div>
						<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
							settled predictive state · strict-prior return resolution
						</div>
					</div>
					<div
						data-scope="symbol"
						data-filter={focusSymbol}
						className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]"
					>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">energy</span>
							<Typography.Span
								data-paint="energy"
								data-paint-absent="—"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">surprise</span>
							<Typography.Span
								data-paint="surprise"
								data-paint-absent="—"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">base alpha</span>
							<Typography.Span
								data-paint="alpha"
								data-paint-absent="—"
								data-paint-format=".4f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">horizon</span>
							<Typography.Span
								data-paint="forecast.supportedHorizon"
								data-paint-absent="—"
								data-paint-suffix=" ticks"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">reach</span>
							<Typography.Span
								data-paint="forecast.probeHorizon"
								data-paint-absent="—"
								data-paint-suffix=" ticks"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">samples</span>
							<Typography.Span
								data-paint="samples"
								data-paint-absent="—"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">task skill</span>
							<Typography.Span
								data-paint="taskSkill"
								data-paint-absent="—"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">forward t+1</span>
							<Typography.Span
								data-paint="taskForecast"
								data-paint-absent="—"
								data-paint-format=".8f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">task scale</span>
							<Typography.Span
								data-paint="taskScale"
								data-paint-absent="—"
								data-paint-format=".8f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
					</div>
					<div data-scope="symbol" data-filter={focusSymbol} className="mt-0.5">
						<div className="mb-1 flex justify-between text-[10px]">
							<span className="text-(--f3)">relative precision</span>
							<Typography.Span
								data-paint="taskRelativePrecision"
								data-paint-absent="—"
								data-paint-format=".3f"
								className="font-mono text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
					</div>
				</div>
			)}
		</Component>
	);
};
