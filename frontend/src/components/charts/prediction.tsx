import { useSelector } from "@tanstack/react-store";
import type { CSSProperties } from "react";
import { atoms, signals } from "#/collections/app";
import { semanticLayerName } from "#/components/terminal/xray-layers";
import { asFinite } from "#/lib/format";
import {
	parseResonanceData,
	type ResonanceData,
	resonanceAt,
} from "#/lib/resonance";
import { Flex } from "../ui";

const vectorSlotTransform = (slot: number, slotCount: number): string =>
	`translateX(${(slot / slotCount) * 100}%) scaleX(${1 / slotCount})`;

const signedVectorTransform = "scaleY(calc(var(--value, 0) * -1))";

interface VectorBarStyle extends CSSProperties {
	"--value": number;
}

const vectorBarStyle = (value: number): VectorBarStyle => ({
	transform: signedVectorTransform,
	"--value": value,
});

const fmt = (value: number | undefined | null, digits: number): string => {
	const n = asFinite(value);
	return n === null ? "—" : n.toFixed(digits);
};

const useResonance = (): ResonanceData | undefined => {
	const focus = useSelector(atoms.focus);
	const row = useSelector(signals.resonance, (state) =>
		resonanceAt(state, focus),
	);
	return row ? parseResonanceData(row) : undefined;
};

const taskCalibration = (res: ResonanceData | undefined): string => {
	if (!res) return "—";
	return res.calibrated ? "CALIBRATED" : "CALIBRATING";
};

const taskSkillStatus = (res: ResonanceData | undefined): string => {
	if (!res) return "—";
	return res.taskSkill !== undefined ? "READY" : "CALIBRATING";
};

const ScalarDiagnostics = () => {
	const res = useResonance();
	const horizon = res?.supportedHorizon;
	const reach = res?.forwardCurve.length;

	return (
		<div className="grid grid-cols-6 gap-px overflow-hidden border border-(--line) bg-(--line)">
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					relative precision
				</div>
				<div data-p="prec" className="mt-0.5 font-mono text-[11px] text-(--up)">
					{fmt(res?.taskRelativePrecision, 3)}
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
					{fmt(res?.taskSkill, 3)}
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
					{res?.surpriseRegimeZ !== undefined
						? `${fmt(res.surprise, 2)} z${fmt(res.surpriseRegimeZ, 2)}`
						: fmt(res?.surprise, 2)}
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
					{fmt(res?.energy, 2)}
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
					regime z
				</div>
				<div
					data-p="regime"
					className="mt-0.5 font-mono text-[11px] text-(--acc)"
				>
					{fmt(res?.surpriseRegimeZ, 2)}
				</div>
			</div>
		</div>
	);
};

const VerdictRow = () => {
	const res = useResonance();

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
					free energy
				</div>
				<div className="flex items-center gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--warning)" />
					<span
						data-p="dynamics"
						className="truncate font-mono text-[13px] font-semibold text-(--warning)"
					>
						{res?.surprise !== undefined && res.energy !== undefined
							? `SURPRISE ${fmt(res.surprise, 2)} · ENERGY ${fmt(res.energy, 2)}`
							: "—"}
					</span>
				</div>
			</div>
		</div>
	);
};

const maxAbsExtent = (values: number[]): number =>
	Math.max(...values.map((value) => Math.abs(value)), Number.EPSILON);

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
		spositionId: `ghost-${label}-slot-${slotIndex}`,
		value,
		index: slotIndex,
		total: ghost.length,
	}));
	const stateSlots = values.map((value, slotIndex) => ({
		spositionId: `state-${label}-slot-${slotIndex}`,
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
								key={slot.spositionId}
								className="absolute inset-y-0 right-1 left-1 origin-left"
								style={{
									transform: vectorSlotTransform(slot.index, slot.total),
								}}
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
						key={slot.spositionId}
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

const HierarchyLanes = () => {
	const res = useResonance();
	const layersData = res?.layers ?? [];
	const layerCount = layersData.length;

	const layers = layersData
		.map((layer, index) => ({
			label: `L${index} · ${semanticLayerName(index, layerCount)}`,
			meta:
				index < layerCount - 1 ? "adjacent generative link" : "context state",
			color: "bg-(--f3)",
			values: layer.state,
			ghost: layer.prediction,
		}))
		.filter((layer) => layer.values.length > 0);

	return (
		<>
			{layers.map((layer) => (
				<VectorLane key={layer.label} {...layer} />
			))}
			<VectorLane
				label="Latent state z"
				meta="settled predictive state · zero centered"
				color="bg-(--warning)"
				values={res?.latent ?? []}
			/>
			<VectorLane
				label="Forward direction shape"
				meta="signed direction lean · t+1 → t+k"
				color="bg-(--acc)"
				values={res?.forwardCurve ?? []}
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
