import type { CSSProperties } from "react";
import { cn } from "#/lib/utils";

export const vectorSlotTransform = (slot: number, slotCount: number): string =>
	`translateX(${(slot / slotCount) * 100}%) scaleX(${1 / slotCount})`;

export const signedVectorTransform = "scaleY(calc(var(--value, 0) * -1))";

interface VectorBarStyle extends CSSProperties {
	"--value": number;
}

export const vectorBarStyle = (value: number): VectorBarStyle => ({
	transform: signedVectorTransform,
	"--value": value,
});

export const fmt = (value: number | undefined | null, digits: number): string =>
	value === undefined || value === null || !Number.isFinite(value)
		? "—"
		: value.toFixed(digits);

export const dir = (value: number | undefined | null): string => {
	if (value === undefined || value === null) return "—";
	if (value > 0) return "up";
	if (value < 0) return "down";
	return "flat";
};

const LAYER_NAMES = ["sensory", "micro", "meso", "macro"];

export const semanticLayerName = (index: number, count: number): string => {
	if (index <= 0) return LAYER_NAMES[0] ?? "sensory";
	if (index >= count - 1) return "macro";
	if (count === 3) return "micro";
	return LAYER_NAMES[index] ?? "latent";
};

export const toVector = (
	value: Float64Array | number[] | null | undefined,
): number[] => (value === null || value === undefined ? [] : Array.from(value));

export const maxAbsExtent = (values: number[]): number =>
	Math.max(...values.map((value) => Math.abs(value)), Number.EPSILON);

const readBool = (obj: any, key: string): boolean => {
	if (!obj) return false;
	if (typeof obj[key] === "function") return Boolean(obj[key]());
	return Boolean(obj[key]);
};

const readNum = (obj: any, key: string): number | undefined => {
	if (!obj) return undefined;
	if (typeof obj[key] === "function") {
		const val = obj[key]();
		return typeof val === "number" ? val : undefined;
	}
	if (typeof obj[key] === "number") return obj[key];
	if (typeof obj[key] === "bigint") return Number(obj[key]);
	if (Array.isArray(obj.metrics)) {
		const metric = obj.metrics.find((m: any) => m?.name === key);
		if (metric && typeof metric.raw === "number") return metric.raw;
	}
	return undefined;
};

const readString = (obj: any, key: string): string | undefined => {
	if (!obj) return undefined;
	if (typeof obj[key] === "function") {
		const val = obj[key]();
		return typeof val === "string" ? val : undefined;
	}
	if (typeof obj[key] === "string") return obj[key];
	return undefined;
};

export const taskCalibration = (artifact: any): string => {
	const calString = readString(artifact, "taskCalibration");
	if (calString) return calString;
	return readBool(artifact, "calibrated") ? "calibrated" : "calibrating";
};

export const taskSkillStatus = (artifact: any): string => {
	const statusString = readString(artifact, "taskSkillStatus");
	if (statusString) return statusString;

	if (!readBool(artifact, "taskSkillReady")) return "calibrating";

	const skill = readNum(artifact, "taskSkill") ?? 0;
	if (skill > 1) return "above baseline";
	if (skill >= 0.5) return "baseline";
	return "below baseline";
};

export const horizonCall = (artifact: any): number | null => {
	if (!artifact) return null;
	if (typeof artifact.forwardCurveLength === "function") {
		const length = artifact.forwardCurveLength();
		return length === 0 ? null : artifact.forwardCurve(length - 1);
	}
	if (Array.isArray(artifact.forwardCurve)) {
		return artifact.forwardCurve.length === 0
			? null
			: artifact.forwardCurve[artifact.forwardCurve.length - 1];
	}
	return null;
};

export interface VectorLaneProps {
	values: number[];
	ghost?: number[];
	label: string;
	meta: string;
	color: string;
	className?: string;
}

export const VectorLane = ({
	values,
	ghost,
	label,
	meta,
	color,
	className,
}: VectorLaneProps) => {
	const stateExtent = maxAbsExtent(values);
	const ghostExtent =
		ghost === undefined ? Number.EPSILON : maxAbsExtent(ghost);

	return (
		<div className={cn("flex min-h-0 flex-1 items-stretch gap-3", className)}>
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

export interface PredictionLayer {
	label?: string;
	meta?: string;
	values?: number[];
	ghost?: number[];
	state?: number[];
	prediction?: number[];
	color?: string;
}

export interface HierarchyLanesProps {
	artifact?: any;
	latent?: number[];
	forwardCurve?: number[];
	layers?: PredictionLayer[] | any;
}

export const HierarchyLanes = ({
	artifact,
	latent: propLatent,
	forwardCurve: propForwardCurve,
	layers: propLayers,
}: HierarchyLanesProps = {}) => {
	const res = artifact;

	let layerItems: Array<{
		label: string;
		meta: string;
		color: string;
		values: number[];
		ghost?: number[];
	}> = [];

	if (Array.isArray(propLayers) && propLayers.length > 0) {
		layerItems = propLayers.map((layer, index) => ({
			label: layer.label ?? `L${index} · ${semanticLayerName(index, propLayers.length)}`,
			meta: layer.meta ?? (index < propLayers.length - 1 ? "adjacent generative link" : "context state"),
			color: layer.color ?? "bg-(--f3)",
			values: toVector(layer.values ?? layer.state),
			ghost: layer.ghost ? toVector(layer.ghost) : layer.prediction ? toVector(layer.prediction) : undefined,
		}));
	} else if (res) {
		const layerCount =
			typeof res.layersLength === "function"
				? res.layersLength()
				: Array.isArray(res.layers)
					? res.layers.length
					: 0;

		layerItems = Array.from({ length: layerCount }, (_, index) => {
			let stateVec: number[] = [];
			let predVec: number[] = [];

			if (typeof res.layers === "function") {
				const layer = res.layers(index);
				stateVec = toVector(layer?.stateArray ? layer.stateArray() : layer?.state);
				predVec = toVector(layer?.predictionArray ? layer.predictionArray() : layer?.prediction);
			} else if (Array.isArray(res.layers)) {
				const layer = res.layers[index];
				stateVec = toVector(layer?.state);
				predVec = toVector(layer?.prediction);
			}

			return {
				label: `L${index} · ${semanticLayerName(index, layerCount)}`,
				meta: index < layerCount - 1 ? "adjacent generative link" : "context state",
				color: "bg-(--f3)",
				values: stateVec,
				ghost: predVec,
			};
		});
	}

	if (layerItems.length === 0) {
		layerItems = [
			{
				label: "L0 · sensory",
				meta: "adjacent generative link",
				color: "bg-(--f3)",
				values: [],
			},
			{
				label: "L1 · micro",
				meta: "context state",
				color: "bg-(--f3)",
				values: [],
			},
		];
	}

	const latentVec =
		propLatent !== undefined
			? toVector(propLatent)
			: res
				? typeof res.latentArray === "function"
					? toVector(res.latentArray())
					: toVector(res.latent)
				: [];

	const forwardVec =
		propForwardCurve !== undefined
			? toVector(propForwardCurve)
			: res
				? typeof res.forwardCurveArray === "function"
					? toVector(res.forwardCurveArray())
					: toVector(res.forwardCurve)
				: [];

	return (
		<>
			{layerItems.map((layer) => (
				<VectorLane key={layer.label} {...layer} />
			))}
			<VectorLane
				label="Latent state z"
				meta="settled predictive state · zero centered"
				color="bg-(--info)"
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

export interface ScalarDiagnosticsProps {
	artifact?: any;
	relativePrecision?: number;
	skill?: number;
	issued?: number;
	realized?: number;
	error?: number;
	horizon?: number;
	reach?: number;
	samples?: number | string;
	surprise?: number;
	energy?: number;
	confidence?: number;
}

export const ScalarDiagnostics = ({
	artifact,
	relativePrecision: propPrec,
	skill: propSkill,
	issued: propIssued,
	realized: propRealized,
	error: propError,
	horizon: propHorizon,
	reach: propReach,
	samples: propSamples,
	surprise: propSurprise,
	energy: propEnergy,
	confidence: propConfidence,
}: ScalarDiagnosticsProps = {}) => {
	const res = artifact;

	const prec = propPrec ?? readNum(res, "taskRelativePrecision");
	const skill = propSkill ?? readNum(res, "taskSkill");
	const issued = propIssued ?? readNum(res, "lastResolutionPrediction");
	const realized = propRealized ?? readNum(res, "lastResolutionTarget");
	const error = propError ?? readNum(res, "lastResolutionError");
	const horizon = propHorizon ?? readNum(res, "supportedHorizon");
	const reach =
		propReach ??
		(res
			? typeof res.forwardCurveLength === "function"
				? res.forwardCurveLength()
				: Array.isArray(res.forwardCurve)
					? res.forwardCurve.length
					: undefined
			: undefined);
	const samples =
		propSamples ??
		(res
			? typeof res.resolvedSteps === "function"
				? String(res.resolvedSteps())
				: res.resolvedSteps !== undefined
					? String(res.resolvedSteps)
					: "—"
			: "—");
	const surprise = propSurprise ?? readNum(res, "surprise");
	const energy = propEnergy ?? readNum(res, "energy");
	const confidence = propConfidence ?? readNum(res, "confidence");

	return (
		<div className="grid grid-cols-5 gap-px overflow-hidden border border-(--line) bg-(--line)">
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
					issued t
				</div>
				<div
					data-p="issued"
					className="mt-0.5 font-mono text-[11px] text-(--f2)"
				>
					{dir(issued)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					realized t+1
				</div>
				<div
					data-p="realized"
					className="mt-0.5 font-mono text-[11px] text-(--f2)"
				>
					{dir(realized)}
				</div>
			</div>
			<div className="bg-(--sunken) px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					forecast error
				</div>
				<div
					data-p="error"
					className="mt-0.5 font-mono text-[11px] text-(--f2)"
				>
					{fmt(error, 0)}
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
					className="mt-0.5 truncate font-mono text-[11px] text-(--info)"
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
		</div>
	);
};

export interface VerdictRowProps {
	artifact?: any;
	status?: string;
	skillStatus?: string;
	forecast?: number;
}

export const VerdictRow = ({
	artifact,
	status,
	skillStatus,
	forecast,
}: VerdictRowProps = {}) => {
	const res = artifact;

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
						{status ?? (res ? taskCalibration(res) : "—")}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-(--sunken) px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					direction skill
				</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span
						data-p="skillStatus"
						className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)"
					>
						{skillStatus ?? (res ? taskSkillStatus(res) : "—")}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-(--sunken) px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					forecast
				</div>
				<div className="flex items-center gap-2">
					<span className="inline-block shrink-0 text-[15px] leading-none text-(--acc)">
						▶
					</span>
					<span
						data-p="forecast"
						className="truncate font-mono text-[13px] text-(--acc)"
					>
						{forecast !== undefined
							? dir(forecast)
							: res
								? dir(horizonCall(res))
								: "—"}
					</span>
				</div>
			</div>
		</div>
	);
};

export interface PredictionChartProps {
	artifact?: any;
	latent?: number[];
	forwardCurve?: number[];
	layers?: PredictionLayer[] | any;
	skill?: number;
	relativePrecision?: number;
	issued?: number;
	realized?: number;
	error?: number;
	horizon?: number;
	reach?: number;
	samples?: number | string;
	surprise?: number;
	energy?: number;
	confidence?: number;
	status?: string;
	skillStatus?: string;
	forecast?: number;
	className?: string;
}

export const PredictionChart = ({
	artifact,
	latent,
	forwardCurve,
	layers,
	skill,
	relativePrecision,
	issued,
	realized,
	error,
	horizon,
	reach,
	samples,
	surprise,
	energy,
	confidence,
	status,
	skillStatus,
	forecast,
	className,
}: PredictionChartProps = {}) => (
	<div className={cn("flex size-full flex-col gap-3 px-4 pt-14 pb-3", className)}>
		<VerdictRow
			artifact={artifact}
			status={status}
			skillStatus={skillStatus}
			forecast={forecast}
		/>
		<ScalarDiagnostics
			artifact={artifact}
			relativePrecision={relativePrecision}
			skill={skill}
			issued={issued}
			realized={realized}
			error={error}
			horizon={horizon}
			reach={reach}
			samples={samples}
			surprise={surprise}
			energy={energy}
			confidence={confidence}
		/>
		<HierarchyLanes
			artifact={artifact}
			latent={latent}
			forwardCurve={forwardCurve}
			layers={layers}
		/>
	</div>
);

export { PredictionChart as TerminalPredictionChart };
