import type { ReactNode } from "react";
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
						resolved samples
					</div>
					<div
						data-paint="samples"
						data-paint-format=".0f"
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
VerdictRow answers the three questions the panel exists to answer, in words,
above every number it is derived from.

Health and direction are deliberately different languages. The two health tiles
carry a word and a tone from --info/--warn/--error and never an arrow; the
forecast tile carries an arrow and the --up/--down pair and never a health tone.
Reusing one green for "the model is fine" and "price is rising" would make a
glance ambiguous in exactly the moment a glance is all there is time for.

Every label is decided by the solver. Nothing here re-reads a threshold.
*/
const VerdictTile = ({
	title,
	label,
	health,
	children,
}: {
	title: string;
	label: string;
	health: string;
	children?: ReactNode;
}) => (
	<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
		<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
			{title}
		</div>
		{/*
			The tone lands on the row and both marks inherit it. An element
			carrying data-set and data-paint together resolves to one binding,
			keyed on the painted path but running in set mode.
		*/}
		<div
			data-set={health}
			data-set-scale="health-color"
			data-target="style.--tone"
			className="flex items-baseline gap-2"
		>
			<span className="size-1.5 shrink-0 self-center rounded-full bg-(--tone,var(--f4))" />
			<span
				data-paint={label}
				className="truncate font-mono text-[13px] uppercase tracking-wide text-(--tone,var(--f3))"
			>
				—
			</span>
		</div>
		{children}
	</div>
);

/*
ForecastArrow is one triangle that rotates: up, flat, or down.

Direction is a sign, so it is painted as rotation rather than as three swapped
glyphs — a mark that turns is read without being re-identified. Conviction fades
it, which keeps an unsupported call from looking as loud as a supported one.
*/
const ForecastArrow = () => (
	<div
		data-set="verdict.direction"
		data-set-scale="sign-color"
		data-target="style.--tone"
		className="flex items-center gap-2"
	>
		<span
			data-set="verdict.direction"
			data-target="style.--dir"
			className="inline-block shrink-0 text-[15px] leading-none text-(--tone,var(--f3))"
			style={{ transform: "rotate(calc((1 - var(--dir, 0)) * 90deg))" }}
		>
			▲
		</span>
		<span
			data-paint="forecast.expectedReturn"
			data-paint-format=".3%"
			className="truncate font-mono text-[13px] text-(--tone,var(--f3))"
		>
			—
		</span>
	</div>
);

const VerdictRow = () => (
	<Component registerKey={RESONANCE_FOCUS}>
		{({ ref }) => (
			<div
				ref={ref}
				className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)"
			>
				<VerdictTile
					title="predictive coding"
					label="verdict.learning"
					health="verdict.learningHealth"
				/>
				<VerdictTile
					title="return learner"
					label="verdict.tuning"
					health="verdict.tuningHealth"
				/>
				<div
					data-set="verdict.conviction"
					data-target="style.--conv"
					className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2"
					style={{ opacity: "calc(0.4 + 0.6 * var(--conv, 0))" }}
				>
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						forecast
					</div>
					<ForecastArrow />
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
	ghost,
}: {
	/* Path inside the focused resonance row. */
	select: string;
	label: string;
	meta: string;
	color: string;
	scale?: "max-abs";
	/*
		Sibling path holding what this lane's vector was predicted to be. Drawn
		full-slot behind the narrower settled bar so the residual reads as the
		exposed shoulder of the ghost rather than as a number to subtract by eye.
	*/
	ghost?: string;
}) => (
	<Component registerKey={RESONANCE_FOCUS} select={select}>
		{({ ref, slots }) => (
			<div ref={ref} className="flex min-h-0 flex-1 items-stretch gap-3">
				<div className="flex w-36 shrink-0 flex-col justify-center gap-0.5 font-mono text-[9px] leading-tight">
					<span className="font-semibold uppercase tracking-widest text-(--f3)">
						{label}
					</span>
					<span className="text-(--f4)">{meta}</span>
				</div>
				<div className="relative min-h-0 flex-1 overflow-hidden border border-(--line) bg-[linear-gradient(to_bottom,transparent_calc(50%-0.5px),var(--line2)_calc(50%-0.5px),var(--line2)_calc(50%+0.5px),transparent_calc(50%+0.5px))]">
					{ghost === undefined ? null : <GhostLane select={ghost} />}
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
								className={`absolute top-1/2 right-1.5 left-1 h-[calc(50%-1px)] origin-top ${color}`}
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
GhostLane draws the top-down prediction for the lane it sits inside.

It is a nested Component rather than a second field on the parent because the
prediction is its own vector on the wire, and the slot count of one vector must
not be inferred from the other's.
*/
const GhostLane = ({ select }: { select: string }) => (
	<Component registerKey={RESONANCE_FOCUS} select={select}>
		{({ ref, slots }) => (
			<div ref={ref} className="absolute inset-0">
				{slots.map((slot) => (
					<div
						key={`${select}-${slot}`}
						data-index={slot}
						className="absolute inset-y-0 right-1 left-1 origin-left"
						style={{ transform: vectorSlotTransform(slot, slots.length) }}
					>
						<div
							data-set="$"
							data-set-scale="max-abs"
							data-target="style.--value"
							className="absolute top-1/2 right-px left-0 h-[calc(50%-1px)] origin-top bg-(--line2)"
							style={{ transform: "scaleY(var(--value, 0))" }}
						/>
					</div>
				))}
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
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-3">
		<VerdictRow />
		<ScalarDiagnostics />
		{HIERARCHY.map((layer) => (
			<VectorLane
				key={layer.label}
				select={`layers.${layer.index}.state`}
				ghost={`layers.${layer.index}.prediction`}
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
