import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import { InspectorMeter } from "./meter";

export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const measurements = useSelector(measurementsStore, (state) => state);
	const source = selectedSource === "prediction" ? "resonance" : selectedSource;
	const history = measurements.measurements[focusSymbol]?.[source]?.values() ?? [];
	const measurement = history.at(-1);
	const category = measurement?.categories.at(0);
	const metrics = measurement?.metrics ?? {};
	const confidence = category?.confidence ?? 0;
	const surprise = category?.surprisal ?? 0;
	const strength = category?.strength ?? 0;
	const status = (
		measurement === undefined
			? "waiting"
			: measurement.status === "fault" ||
					measurement.status === "ambiguous" ||
					measurement.status === "calibrating"
				? measurement.status
				: "measured"
	) as SignalHealthStatus;
	const copy = kernelCopy(selectedSource, category?.type ?? selectedSource);
	const statusMeta = kernelStatusMeta(status);
	const observedAt = measurement?.at;
	const observedStamp =
		typeof observedAt === "number"
			? observedAt
			: typeof observedAt === "string" && observedAt.trim() !== ""
				? Date.parse(observedAt)
				: Number.NaN;

	return (
		<div className="min-h-0 overflow-auto px-5 py-[18px]">
			<div className="flex items-start justify-between gap-3">
				<div>
					<h2 className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
						{copy.name}
					</h2>
					<div className="mt-1 font-mono text-[11px] text-(--f3)">
						{copy.sub}
					</div>
				</div>
				<span
					className="shrink-0 rounded-[2px] border px-2.5 py-1 text-[11px] font-semibold tracking-wider uppercase"
					style={{
						borderColor: statusMeta.bd,
						backgroundColor: statusMeta.bg,
						color: statusMeta.fg,
					}}
				>
					{statusMeta.label}
				</span>
			</div>
			<p className="mt-3.5 max-w-[560px] font-serif text-[15px] text-(--f2) leading-[1.55]">
				{copy.blurb}
			</p>
			{measurement === undefined ? (
				<div className="mt-[18px] rounded border border-(--line) bg-(--sunken) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
					waiting for backend {selectedSource} measurement
				</div>
			) : null}
			{measurement === undefined ? null : (
				<div className="mt-[18px] grid grid-cols-2 gap-x-[22px] gap-y-3">
					<InspectorMeter
						label="Confidence"
						value={`${Math.floor(confidence * 100)}%`}
						percent={confidence * 100}
						color="var(--info)"
					/>
					<InspectorMeter
						label="Surprise"
						value={`${surprise.toFixed(2)}x`}
						percent={Math.min(100, surprise * 100)}
						color={status === "ambiguous" ? "var(--acc)" : "var(--info)"}
					/>
					<InspectorMeter
						label="Strength"
						value={strength.toFixed(4)}
						percent={strength * 100}
						color="var(--up)"
					/>
					<InspectorMeter
						label="Class conf"
						value={`${Math.floor(Number(metrics.cognitiveClassConfidence) * 100 || 0)}%`}
						percent={Math.floor(Number(metrics.cognitiveClassConfidence) * 100 || 0)}
						color="var(--info)"
					/>
				</div>
			)}
			<div className="mt-5 grid grid-cols-2 gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
				<div className="flex justify-between">
					<span className="text-(--f3)">Samples</span>
					<span className="text-(--f1)">{history.length.toLocaleString()}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Strength</span>
					<span className="text-(--f1)">{strength.toFixed(4)}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Observed</span>
					<span className="text-(--f1)">
						{Number.isFinite(observedStamp)
							? new Date(observedStamp).toLocaleTimeString("en-US", {
									hour12: false,
								})
							: "— / —"}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Gap</span>
					<span className="text-(--f1)">
						{String(metrics.gap ?? "none")}
					</span>
				</div>
			</div>
		</div>
	);
};
