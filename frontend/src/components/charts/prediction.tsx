import { useSelector } from "@tanstack/react-store";
import { type CSSProperties, useRef } from "react";
import { focusAtom, resonanceStore } from "#/collections/app";
import { semanticLayerName } from "#/components/terminal/xray-layers";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { Flex } from "../ui";

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
	value === undefined || value === null || !Number.isFinite(value)
		? "—"
		: value.toFixed(digits);

export type LayerData = {
	state?: number[];
	prediction?: number[];
};

export type ResonanceData = {
	taskRelativePrecision?: number;
	taskSkill?: number;
	supportedHorizon?: number;
	resolvedSteps?: number;
	surprise?: number;
	energy?: number;
	confidence?: number;
	calibrated?: boolean;
	taskCalibration?: string;
	taskSkillStatus?: string;
	taskSkillReady?: boolean;
	forwardCurve: number[];
	dynamics?: Record<string, number>;
	latent?: number[];
	layers?: LayerData[];
};

export const parseResonanceData = (measurement: MeasurementT | undefined): ResonanceData | undefined => {
	if (!measurement) return undefined;
	
	const data: ResonanceData = { forwardCurve: [], dynamics: {}, latent: [], layers: [] };
	
	for (let i = 0; i < measurement.metrics.length; i++) {
		const m = measurement.metrics[i];
		const lbl = String(m.name);
		if (lbl === "task_relative_precision") data.taskRelativePrecision = m.raw;
		if (lbl === "task_skill") data.taskSkill = m.raw;
		if (lbl === "surprise") data.surprise = m.raw;
		if (lbl === "energy") data.energy = m.raw;
		if (lbl?.startsWith("forward_curve_")) {
			const idx = parseInt(lbl.split("_")[2], 10);
			if (!Number.isNaN(idx)) {
				data.forwardCurve[idx] = m.raw;
			}
		}
		if (lbl?.startsWith("dynamics_")) {
			const prop = lbl.replace("dynamics_", "");
			if (data.dynamics) {
				data.dynamics[prop] = m.raw;
			}
		}
		if (lbl?.startsWith("latent_")) {
			const idx = parseInt(lbl.split("_")[1], 10);
			if (!Number.isNaN(idx) && data.latent) {
				data.latent[idx] = m.raw;
			}
		}
		if (lbl?.startsWith("layer_")) {
			const parts = lbl.split("_");
			const layerId = parseInt(parts[1], 10);
			const prop = parts[2];
			const idx = parseInt(parts[3], 10);
			
			if (!Number.isNaN(layerId) && !Number.isNaN(idx) && data.layers) {
				if (!data.layers[layerId]) {
					data.layers[layerId] = { state: [], prediction: [] };
				}
				const layer = data.layers[layerId];
				if (prop === "state" && layer.state) {
					layer.state[idx] = m.raw;
				} else if (prop === "prediction" && layer.prediction) {
					layer.prediction[idx] = m.raw;
				}
			}
		}
	}
	
	for (let i = 0; i < measurement.provenance.length; i++) {
		const p = measurement.provenance[i];
		const k = String(p.name);
		const v = String(p.value);
		if (k === "supported_horizon") data.supportedHorizon = parseInt(v, 10);
		if (k === "resolved_steps") data.resolvedSteps = parseInt(v, 10);
		if (k === "confidence") data.confidence = parseFloat(v);
		if (k === "calibrated") data.calibrated = v === "true";
	}
	
	return data;
};

const useArtifact = (): ResonanceData | undefined => {
	const symbol = useSelector(focusAtom);

	const row = useSelector(resonanceStore, (state) => {
		const ring = state[symbol];
		return ring && !ring.isEmpty() ? (ring.getLast() ?? undefined) : undefined;
	});

	const held = useRef<{
		symbol: string | undefined;
		row: MeasurementT | undefined;
	}>({ symbol, row: undefined });

	if (held.current.symbol !== symbol) {
		held.current = { symbol, row: undefined };
	}

	if (row !== undefined) {
		held.current.row = row;
	}

	return parseResonanceData(held.current.row);
};

const taskCalibration = (res: ResonanceData | undefined): string => {
	if (!res) return "—";
	if (typeof res.taskCalibration === "string" && res.taskCalibration.length > 0) {
		return res.taskCalibration;
	}
	return res.calibrated ? "CALIBRATED" : "CALIBRATING";
};

const taskSkillStatus = (res: ResonanceData | undefined): string => {
	if (!res) return "—";
	if (typeof res.taskSkillStatus === "string" && res.taskSkillStatus.length > 0) {
		return res.taskSkillStatus;
	}
	return res.taskSkillReady ? "READY" : "CALIBRATING";
};

const ScalarDiagnostics = () => {
	const res = useArtifact();

	const prec = res?.taskRelativePrecision;
	const skill = res?.taskSkill;
	const horizon = res ? Number(res.supportedHorizon) : undefined;
	const reach = res?.forwardCurve.length;
	const samples = res ? String(res.resolvedSteps) : "—";
	const surprise = res?.surprise;
	const energy = res?.energy;
	const confidence = res?.confidence;

	return (
		<div className="grid grid-cols-7 gap-px overflow-hidden border border-(--line) bg-(--line)">
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					relative precision
				</div>
				<div data-p="prec" className="mt-0.5 font-mono text-[11px] text-(--up)">
					{fmt(prec, 3)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					task skill
				</div>
				<div
					data-p="skill"
					className="mt-0.5 font-mono text-[11px] text-(--f2)"
				>
					{fmt(skill, 3)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					surprise
				</div>
				<div
					data-p="surprise"
					className="mt-0.5 truncate font-mono text-[11px] text-(--warning)"
				>
					{fmt(surprise, 2)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					energy
				</div>
				<div
					data-p="energy"
					className="mt-0.5 truncate font-mono text-[11px] text-(--warning)"
				>
					{fmt(energy, 2)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					confidence
				</div>
				<div
					data-p="confidence"
					className="mt-0.5 truncate font-mono text-[11px] text-(--f2)"
				>
					{fmt(confidence, 3)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					horizon / reach
				</div>
				<div className="mt-0.5 flex gap-1 font-mono text-[11px] text-(--f2)">
					<span data-p="horizon">{fmt(horizon, 0)}</span>
					<span>/</span>
					<span data-p="reach">{fmt(reach, 0)}</span>
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					resolved samples
				</div>
				<div
					data-p="samples"
					className="mt-0.5 font-mono text-[11px] text-(--acc)"
				>
					{samples}
				</div>
			</div>
		</div>
	);
};

const VerdictRow = () => {
	const res = useArtifact();
	const surpriseVal = res?.surprise;
	const energyVal = res?.energy;

	return (
		<div className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
			<div className="flex flex-col justify-between gap-1.5 bg-(--sunken) px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					residual model
				</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span
						data-p="calibration"
						className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)"
					>
						{taskCalibration(res)}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-(--sunken) px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					representation skill
				</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span
						data-p="skillStatus"
						className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)"
					>
						{taskSkillStatus(res)}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-(--sunken) px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					free energy dynamics
				</div>
				<div className="flex items-center gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--warning)" />
					<span
						data-p="dynamics"
						className="truncate font-mono text-[13px] font-semibold text-(--warning)"
					>
						{surpriseVal !== undefined && energyVal !== undefined
							? `SURPRISE ${fmt(surpriseVal, 2)} · ENERGY ${fmt(energyVal, 2)}`
							: "—"}
					</span>
				</div>
			</div>
		</div>
	);
};

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
	const ghostExtent =
		ghost === undefined ? Number.EPSILON : maxAbsExtent(ghost);
	const ghostSlots = ghost?.map((value, slotIndex) => ({
		slotId: `ghost-${label}-slot-${slotIndex}`,
		value,
		index: slotIndex,
		total: ghost.length,
	}));
	const stateSlots = values.map((value, slotIndex) => ({
		slotId: `state-${label}-slot-${slotIndex}`,
		value,
		index: slotIndex,
		total: values.length,
	}));

	return (
		<div className="flex min-h-0 flex-1 items-stretch gap-3">
			<div className="flex w-36 shrink-0 flex-col justify-center gap-0.5 font-mono text-[9px] leading-tight">
				<span className="font-semibold uppercase tracking-widest text-(--f3)">
					{label}
				</span>
				<span className="text-(--f4)">{meta}</span>
			</div>
			<div className="relative min-h-0 flex-1 overflow-hidden border border-(--line) bg-[linear-gradient(to_bottom,transparent_calc(50%-0.5px),var(--line2)_calc(50%-0.5px),var(--line2)_calc(50%+0.5px),transparent_calc(50%+0.5px))]">
				{ghostSlots !== undefined ? (
					<div className="absolute inset-0">
						{ghostSlots.map((slot) => (
							<div
								key={slot.slotId}
								className="absolute inset-y-0 right-1 left-1 origin-left"
								style={{ transform: vectorSlotTransform(slot.index, slot.total) }}
							>
								<div
									className="absolute top-1/2 right-px left-0 h-[calc(50%-1px)] origin-top bg-(--line2)"
									style={vectorBarStyle(slot.value / ghostExtent)}
								/>
							</div>
						))}
					</div>
				) : null}
				{stateSlots.map((slot) => (
					<div
						key={slot.slotId}
						className="absolute inset-y-0 right-1 left-1 origin-left"
						style={{ transform: vectorSlotTransform(slot.index, slot.total) }}
					>
						<div
							className={`absolute top-1/2 right-1.5 left-1 h-[calc(50%-1px)] origin-top ${color}`}
							style={vectorBarStyle(slot.value / stateExtent)}
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
	const layersData = res?.layers ?? [];
	const layerCount = layersData.length;

	const layers = layersData.map((layer, index) => ({
		label: `L${index} · ${semanticLayerName(index, layerCount)}`,
		meta:
			index < layerCount - 1 ? "adjacent generative link" : "context state",
		color: "bg-(--f3)",
		values: layer.state ?? [],
		ghost: layer.prediction ?? [],
	}));

	const latentVec = res?.latent ?? [];
	const forwardVec = res?.forwardCurve ?? [];

	return (
		<>
			{layers.map((layer) => (
				<VectorLane key={layer.label} {...layer} />
			))}
			<VectorLane
				label="Latent state z"
				meta="settled predictive state · zero centered"
				color="bg-(--warning)"
				values={latentVec}
			/>
			<VectorLane
				label="Forward direction shape"
				meta="signed direction lean · t+1 → t+k"
				color="bg-(--acc)"
				values={forwardVec}
			/>
		</>
	);
};

export const TerminalPredictionChart = () => (
	<Flex.Column gap={3} className="size-full px-4 pt-14 pb-3">
		<VerdictRow />
		<ScalarDiagnostics />
		<HierarchyLanes />
	</Flex.Column>
);
