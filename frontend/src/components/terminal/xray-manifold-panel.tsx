import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "../ui";

const ROWS = [
	{
		label: "energy",
		paint: "energy",
		format: ".3f",
	},
	{
		label: "surprise",
		paint: "surprise",
		format: ".3f",
	},
	{
		label: "base alpha",
		paint: "alpha",
		format: ".4f",
	},
	{
		label: "horizon",
		paint: "forecast.supportedHorizon",
		suffix: " ticks",
	},
	{
		label: "reach",
		paint: "forecast.probeHorizon",
		suffix: " ticks",
	},
	{
		label: "samples",
		paint: "samples",
	},
	{
		label: "task skill",
		paint: "taskSkill",
	},
	{
		label: "direction t+1",
		paint: "taskForecast",
		format: "dir",
	},
	{
		label: "candidate",
		paint: "taskCandidate",
		format: "dir",
	},
]

const DYNAMICS_FIELDS = [
	{ label: "velocity", path: "dynamics.velocity", format: ".4f" },
	{ label: "acceleration", path: "dynamics.acceleration", format: ".4f" },
	{ label: "liquid memory", path: "dynamics.memory", format: ".4f" },
	{ label: "memory scale", path: "dynamics.memoryScale", format: ".4f" },
	{ label: "stored energy", path: "dynamics.storedEnergy", format: ".4f" },
	{ label: "supplied power", path: "dynamics.suppliedPower", format: ".4f" },
	{ label: "dissipation", path: "dynamics.dissipation", format: ".4f" },
	{
		label: "passivity residue",
		path: "dynamics.passivityResidue",
		format: ".4f",
	},
	{
		label: "diffusion variance",
		path: "dynamics.continuousVariance",
		format: ".6f",
	},
	{ label: "jump amplitude", path: "dynamics.jumpAmplitude", format: ".6f" },
	{ label: "jump variance", path: "dynamics.jumpVariance", format: ".6f" },
	{ label: "rotor norm", path: "dynamics.equivarianceNorm", format: ".4f" },
] as const;

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
				<Flex.Column
					gap={2}
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
							settled predictive state · strict-prior direction resolution
						</div>
					</div>
					<div
						data-scope="symbol"
						data-filter={focusSymbol}
						className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]"
					>
						{ROWS.map((row) => (
							<Flex.Row key={row.label} justify="between" gap={3}>
								<span className="text-(--f3)">{row.label}</span>
								<Typography.Span
									data-paint={row.paint}
									data-paint-absent="—"
									data-paint-format={row.format}
									className="text-right text-(--f1)"
								>
									—
								</Typography.Span>
							</Flex.Row>
						))}
						<Flex.Row justify="between" gap={3}>
							<span className="text-(--f3)">stable / held</span>
							<span className="text-right text-(--f1)">
								<Typography.Span
									data-paint="taskStable"
									data-paint-absent="—"
									data-paint-format="dir"
								>
									—
								</Typography.Span>
								{" / "}
								<Typography.Span
									data-paint="taskHeld"
									data-paint-absent="false"
								>
									false
								</Typography.Span>
							</span>
						</Flex.Row>
						<Flex.Row justify="between" gap={3}>
							<span className="text-(--f3)">task scale</span>
							<Typography.Span
								data-paint="taskScale"
								data-paint-absent="—"
								data-paint-format=".8f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</Flex.Row>
					</div>
					<div
						data-scope="symbol"
						data-filter={focusSymbol}
						className="mt-1 border-(--line) border-t pt-2"
					>
						<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Continuous dynamics
						</div>
						<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
							{DYNAMICS_FIELDS.map((field) => (
								<div key={field.path} className="flex justify-between gap-3">
									<span className="text-(--f3)">{field.label}</span>
									<Typography.Span
										data-paint={field.path}
										data-paint-absent="—"
										data-paint-format={field.format}
										className="text-right text-(--f1)"
									>
										—
									</Typography.Span>
								</div>
							))}
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
				</Flex.Column>
			)}
		</Component>
	);
};
