import { useSelector } from "@tanstack/react-store";
import type { CSSProperties } from "react";
import { focusStore, resonanceArtifactStore } from "#/collections/app";
import { semanticLayerName } from "#/components/terminal/xray-layers";
import type { EnvelopeResonanceArtifact } from "#/providers/telemetry/telemetry/envelope-resonance-artifact";
import { EnvelopeResonanceLayer } from "#/providers/telemetry/telemetry/envelope-resonance-layer";

export const vectorSlotTransform = (slot: number, slotCount: number): string =>
	`translateX(${(slot / slotCount) * 100}%) scaleX(${1 / slotCount})`;

export const signedVectorTransform = "scaleY(calc(var(--value, 0) * -1))";

interface VectorBarStyle extends CSSProperties {
	"--value": number;
}

const vectorBarStyle = (value: number): VectorBarStyle => ({
	transform: signedVectorTransform,
	"--value": value,
});

const fmt = (value: number | undefined | null, digits: number): string =>
	value === undefined || value === null || !Number.isFinite(value) ? "—" : value.toFixed(digits);

const dir = (value: number | undefined | null): string => {
	if (value === undefined || value === null) return "—";
	if (value > 0) return "up";
	if (value < 0) return "down";
	return "flat";
};

const layerObj = new EnvelopeResonanceLayer();

/*
The resonance artifact rides every envelope (types.Envelope.Resonance), and the
artifact store is not pre-scoped to one symbol — the solver keys its coder per
symbol across the cross-section — so the focused symbol is selected here, the
same way the other resonance surfaces do it.
*/
const useArtifact = (): EnvelopeResonanceArtifact | undefined => {
	const symbol = useSelector(focusStore, (state) => state);

	return useSelector(resonanceArtifactStore, (state) =>
		state.findLast((row) => row.symbol() === symbol),
	);
};

/*
taskCalibration and taskSkillStatus read the coder's own readiness as words.
They were assembled backend-side when the panel had a curated frame of its own;
with the artifact carrying the raw quantities, the wording belongs here — it is
presentation, and the envelope stays the numbers it measured.
*/
const taskCalibration = (artifact: EnvelopeResonanceArtifact): string =>
	artifact.calibrated() ? "calibrated" : "calibrating";

const taskSkillStatus = (artifact: EnvelopeResonanceArtifact): string => {
	if (!artifact.taskSkillReady()) return "calibrating";

	const skill = artifact.taskSkill();

	if (skill > 1) return "above baseline";
	if (skill >= 0.5) return "baseline";

	return "below baseline";
};

/*
The forward curve is cumulative per horizon: element k predicts the direction of
the move over the next k+1 ticks, so the call for the supported horizon is the
curve's last element.
*/
const horizonCall = (artifact: EnvelopeResonanceArtifact): number | null => {
	const length = artifact.forwardCurveLength();

	return length === 0 ? null : artifact.forwardCurve(length - 1);
};

const ScalarDiagnostics = () => {
	const res = useArtifact();

	return (
		<div className="grid grid-cols-5 gap-px overflow-hidden border border-(--line) bg-(--line)">
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">relative precision</div>
				<div data-p="prec" className="mt-0.5 font-mono text-[11px] text-(--up)">
					{res ? fmt(res.taskRelativePrecision(), 3) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">task skill</div>
				<div data-p="skill" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? fmt(res.taskSkill(), 3) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">issued t</div>
				<div data-p="issued" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? dir(res.lastResolutionTarget()) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">realized t+1</div>
				<div data-p="realized" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? dir(res.lastResolutionTarget()) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">forecast error</div>
				<div data-p="error" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? fmt(res.lastResolutionError(), 0) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">horizon / reach</div>
				<div className="mt-0.5 flex gap-1 font-mono text-[11px] text-(--f2)">
					<span data-p="horizon">{res ? fmt(Number(res.supportedHorizon()), 0) : "—"}</span>
					<span>/</span>
					<span data-p="reach">{res ? fmt(res.forwardCurveLength(), 0) : "—"}</span>
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">resolved samples</div>
				<div data-p="samples" className="mt-0.5 font-mono text-[11px] text-(--acc)">
					{res ? String(res.resolvedSteps()) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">surprise</div>
				<div data-p="surprise" className="mt-0.5 truncate font-mono text-[11px] text-(--warning)">
					{res ? fmt(res.surprise(), 2) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">energy</div>
				<div data-p="energy" className="mt-0.5 truncate font-mono text-[11px] text-(--info)">
					{res ? fmt(res.energy(), 2) : "—"}
				</div>
			</div>
		</div>
	);
};

const VerdictRow = () => {
	const res = useArtifact();

	return (
		<div className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">residual model</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span data-p="calibration" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">
						{res ? taskCalibration(res) : "—"}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">direction skill</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span data-p="skillStatus" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">
						{res ? taskSkillStatus(res) : "—"}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">forecast</div>
				<div className="flex items-center gap-2">
					<span className="inline-block shrink-0 text-[15px] leading-none text-(--acc)">▶</span>
					<span data-p="forecast" className="truncate font-mono text-[13px] text-(--acc)">
						{res ? dir(horizonCall(res)) : "—"}
					</span>
				</div>
			</div>
		</div>
	);
};

const toVector = (value: Float64Array | null | undefined): number[] =>
	value === null || value === undefined ? [] : Array.from(value);

/*
Each lane is normalized against its own largest component because every layer's
state has a different width and magnitude; a shared scale would flatten the
quieter lanes onto the zero line.
*/
const maxAbsExtent = (values: number[]): number =>
	Math.max(...values.map((value) => Math.abs(value)), Number.EPSILON);

/*
VectorLane unrolls one vector into a bar per component straddling a zero line.
The forward curve is legible because bar k is the direction lean k steps out.
An optional ghost vector — the top-down prediction for a layer — is drawn
full-slot behind the narrower settled bar so the residual reads as the exposed
shoulder of the ghost rather than as a number to subtract by eye.
*/
const VectorLane = ({
	values,
	ghost,
	label,
	meta,
	color,
}: {
	values: number[];
	ghost?: number[];
	label: string;
	meta: string;
	color: string;
}) => {
	const stateExtent = maxAbsExtent(values);
	const ghostExtent = ghost === undefined ? Number.EPSILON : maxAbsExtent(ghost);

	return (
		<div className="flex min-h-0 flex-1 items-stretch gap-3">
			<div className="flex w-36 shrink-0 flex-col justify-center gap-0.5 font-mono text-[9px] leading-tight">
				<span className="font-semibold uppercase tracking-widest text-(--f3)">
					{label}
				</span>
				<span className="text-(--f4)">{meta}</span>
			</div>
			<div className="relative min-h-0 flex-1 overflow-hidden border border-(--line) bg-[linear-gradient(to_bottom,transparent_calc(50%-0.5px),var(--line2)_calc(50%-0.5px),var(--line2)_calc(50%+0.5px),transparent_calc(50%+0.5px))]">
				{ghost !== undefined ? (
					<div className="absolute inset-0">
						{ghost.map((value, index) => (
							<div
								// biome-ignore lint/suspicious/noArrayIndexKey: vector slots are positional and never reordered
								key={`ghost-${index}`}
								className="absolute inset-y-0 right-1 left-1 origin-left"
								style={{ transform: vectorSlotTransform(index, ghost.length) }}
							>
								<div
									className="absolute top-1/2 right-px left-0 h-[calc(50%-1px)] origin-top bg-(--line2)"
									style={vectorBarStyle(value / ghostExtent)}
								/>
							</div>
						))}
					</div>
				) : null}
				{values.map((value, index) => (
					<div
						// biome-ignore lint/suspicious/noArrayIndexKey: vector slots are positional and never reordered
						key={`state-${index}`}
						className="absolute inset-y-0 right-1 left-1 origin-left"
						style={{ transform: vectorSlotTransform(index, values.length) }}
					>
						<div
							className={`absolute top-1/2 right-1.5 left-1 h-[calc(50%-1px)] origin-top ${color}`}
							style={vectorBarStyle(value / stateExtent)}
						/>
					</div>
				))}
			</div>
		</div>
	);
};

/*
HierarchyLanes paints every emitted predictive-coding layer as a state/prediction
pair, followed by the settled latent vector and the signed forward-direction
curve. All lanes read the focused carrier row from the resonance store.
*/
const HierarchyLanes = () => {
	const res = useArtifact();

	const layerCount = res ? res.layersLength() : 0;
	const layers = Array.from({ length: layerCount }, (_, index) => {
		const layer = res?.layers(index, layerObj);

		return {
			label: `L${index} · ${semanticLayerName(index, layerCount)}`,
			meta: index < layerCount - 1 ? "adjacent generative link" : "context state",
			color: "bg-(--f3)",
			values: toVector(layer?.stateArray()),
			ghost: toVector(layer?.predictionArray()),
		};
	});
	return (
		<>
			{layers.map((layer) => (
				<VectorLane key={layer.label} {...layer} />
			))}
			<VectorLane
				label="Latent state z"
				meta="settled predictive state · zero centered"
				color="bg-(--info)"
				values={toVector(res?.latentArray())}
			/>
			<VectorLane
				label="Forward direction shape"
				meta="signed direction lean · t+1 → t+k"
				color="bg-(--acc)"
				values={toVector(res?.forwardCurveArray())}
			/>
		</>
	);
};

export const TerminalPredictionChart = () => (
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-3">
		<VerdictRow />
		<ScalarDiagnostics />
		<HierarchyLanes />
	</div>
);