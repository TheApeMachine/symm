import { Component } from "#/components/ui/component";
import { RESONANCE_FOCUS } from "#/providers/ws-stores";

export const vectorSlotTransform = (slot: number, slotCount: number): string =>
	`translateX(${(slot / slotCount) * 100}%) scaleX(${1 / slotCount})`;

/*
Every lane reads the focused-carrier stream, so the whole chart describes one
symbol without each binding having to name it.
*/
const ScalarDiagnostics = () => (
	<Component registerKey={RESONANCE_FOCUS}>
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

/*
VectorLane unrolls one vector into a bar per component.

Each bar is one element straddling a zero line, which is what makes the forward
curve legible: bar k is the return predicted k steps out, so the profile across
the lane is the horizon itself. Nothing is retained and nothing is recomputed —
the websocket writes each bar's own scale factor and React never re-renders.
*/
const VectorLane = ({
	select,
	label,
	meta,
	color,
	scale,
}: {
	/* Path inside the focused resonance row. */
	select: string;
	label: string;
	meta: string;
	color: string;
	scale?: "max-abs";
}) => (
	<Component registerKey={RESONANCE_FOCUS} select={select}>
		{({ ref, slots }) => (
			<div ref={ref} className="flex min-h-0 flex-1 flex-col">
				<div className="mb-1 flex items-center justify-between gap-3 font-mono text-[9px]">
					<span className="font-semibold uppercase tracking-widest text-(--f3)">
						{label}
					</span>
					<span className="text-(--f4)">{meta}</span>
				</div>
				<div className="relative min-h-0 flex-1 overflow-hidden border border-(--line) bg-[linear-gradient(to_bottom,transparent_calc(50%-0.5px),var(--line2)_calc(50%-0.5px),var(--line2)_calc(50%+0.5px),transparent_calc(50%+0.5px))]">
					{slots.map((slot) => (
						<div
							key={`${select}-${slot}`}
							data-index={slot}
							data-paint="$"
							data-paint-prop="title"
							data-paint-format=".4f"
							className="absolute inset-y-0 right-1 left-1 origin-left"
							style={{ transform: vectorSlotTransform(slot, slots.length) }}
						>
							<div
								data-set="$"
								data-set-scale={scale}
								data-target="style.--value"
								className={`absolute top-1/2 right-px left-0 h-[calc(50%-1px)] origin-top ${color}`}
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
The settled hierarchy, sensory layer first.

Each layer's state is a different width and a different magnitude, so each lane
is scaled against its own largest component. Sharing one scale across them would
flatten the quieter layers onto the zero line.
*/
const HIERARCHY = [
	{ index: 0, label: "L0 · sensory", meta: "bottom-up reconstruction" },
	{ index: 1, label: "L1 · micro", meta: "adjacent generative link" },
	{ index: 2, label: "L2 · macro", meta: "context state" },
] as const;

/*
TerminalPredictionChart binds scalar diagnostics and every dynamic vector
directly to the focused row inside the backend resonance batch. Array slots
unroll each vector into granular DOM paint targets; no chart-local websocket
painter or retained JS history is needed.
*/
export const TerminalPredictionChart = () => (
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-7">
		<ScalarDiagnostics />
		{HIERARCHY.map((layer) => (
			<VectorLane
				key={layer.label}
				select={`layers.${layer.index}.state`}
				label={layer.label}
				meta={layer.meta}
				color="bg-(--f3)"
				scale="max-abs"
			/>
		))}
		<VectorLane
			select="latent"
			label="Latent state z"
			meta="settled predictive state · zero centered"
			color="bg-(--info)"
			scale="max-abs"
		/>
		<VectorLane
			select="forecast.forwardCurve"
			label="Forward return curve"
			meta="dynamic recurrent rollout · t+1 → t+k"
			color="bg-(--acc)"
			scale="max-abs"
		/>
	</div>
);
