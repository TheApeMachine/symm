import { Component } from "#/components/ui/component";

const ScalarDiagnostics = () => (
	<Component registerKey="resonance" select="0">
		{({ ref }) => (
			<div
				ref={ref}
				className="grid grid-cols-5 gap-px overflow-hidden border border-(--line) bg-(--line)"
			>
				<div className="bg-[#0a0907] px-2 py-1.5">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						confidence
					</div>
					<div
						data-paint="confidence"
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
						data-paint="activeHorizon"
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

const VectorLane = ({
	select,
	label,
	meta,
	color,
	scale,
}: {
	select: "latent" | "forwardCurve";
	label: string;
	meta: string;
	color: string;
	scale?: "max-abs";
}) => (
	<Component registerKey="resonance" select={`0.${select}`}>
		{({ ref, slots }) => (
			<div ref={ref} className="flex min-h-0 flex-1 flex-col">
				<div className="mb-1 flex items-center justify-between gap-3 font-mono text-[9px]">
					<span className="font-semibold uppercase tracking-widest text-(--f3)">
						{label}
					</span>
					<span className="text-(--f4)">{meta}</span>
				</div>
				<div className="relative flex min-h-0 flex-1 items-stretch gap-px overflow-hidden border border-(--line) bg-[linear-gradient(to_bottom,transparent_calc(50%-0.5px),var(--line2)_calc(50%-0.5px),var(--line2)_calc(50%+0.5px),transparent_calc(50%+0.5px))] px-1">
					{slots.map((slot) => (
						<div
							key={`${select}-${slot}`}
							data-index={slot}
							data-paint="$"
							data-paint-prop="title"
							data-paint-format=".4f"
							className="relative min-w-px flex-1"
						>
							<div
								data-set="$"
								data-set-scale={scale}
								data-target="style.--value"
								className={`absolute top-1/2 right-0 left-0 h-[calc(50%-1px)] origin-top ${color}`}
								style={{ transform: "scaleY(var(--value, 0))" }}
							/>
						</div>
					))}
				</div>
			</div>
		)}
	</Component>
);

/*
TerminalPredictionChart binds scalar diagnostics and both dynamic vectors directly
to the focused row inside the backend resonance batch. Array slots unroll each
vector into granular DOM paint targets; no chart-local websocket painter or
retained JS history is needed.
*/
export const TerminalPredictionChart = () => (
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-7">
		<ScalarDiagnostics />
		<VectorLane
			select="latent"
			label="Latent state z"
			meta="settled predictive state · zero centered"
			color="bg-(--info)"
		/>
		<VectorLane
			select="forwardCurve"
			label="Forward return curve"
			meta="dynamic recurrent rollout · t+1 → t+k"
			color="bg-(--acc)"
			scale="max-abs"
		/>
	</div>
);
