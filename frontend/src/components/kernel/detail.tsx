import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
} from "#/components/terminal/kernel-meta";
import {
	kernelFrameForSource as measurementForSource,
	kernelReadout as readMeasurement,
} from "#/components/terminal/rows";
import { InspectorMeter } from "./meter";

export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const measurements = useSelector(measurementsStore, (state) => state);
	const source = selectedSource === "prediction" ? "resonance" : selectedSource;
	const measurement = measurementForSource(
		measurements,
		selectedSource,
		focusSymbol,
	);
	const { output, confidence, surprise, status, strength } =
		readMeasurement(measurement);
	const copy = kernelCopy(
		selectedSource,
		String(output.category ?? selectedSource),
	);
	const statusMeta = kernelStatusMeta(status);
	const symbols = Object.keys(measurements[source] ?? {}).filter((scope) =>
		scope.includes("/"),
	);
	const active = symbols.filter((symbol) => {
		const row = readMeasurement(measurements[source]?.[symbol]);

		return row.status !== "waiting" && row.status !== "standby";
	}).length;
	const observedAt =
		measurement?.observed_at ??
		output.observed_at ??
		measurement?.ts ??
		output.ts;
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
						value={`${Math.floor(Number(output.cognitiveClassConfidence) * 100 || 0)}%`}
						percent={Math.floor(
							Number(output.cognitiveClassConfidence) * 100 || 0,
						)}
						color="var(--info)"
					/>
				</div>
			)}
			<div className="mt-5 grid grid-cols-2 gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
				<div className="flex justify-between">
					<span className="text-(--f3)">Active measurements</span>
					<span className="text-(--f1)">
						{active.toLocaleString()} / {symbols.length.toLocaleString()}
					</span>
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
						{String(output.gap ?? measurement?.gap ?? "none")}
					</span>
				</div>
			</div>
			<div className="mt-[18px]">
				<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Cross-section · confidence heatmap
				</div>
				{symbols.length === 0 ? (
					<div className="py-6 text-center font-mono text-(--f4) text-xs">
						Waiting for cross-section measurements
					</div>
				) : (
					<div className="grid grid-cols-12 gap-[3px]">
						{symbols.slice(0, 24).map((symbol) => {
							const row = readMeasurement(measurements[source]?.[symbol]);
							const value = Math.min(1, Math.max(0, row.confidence));
							const color =
								value >= 0.72
									? { bg: "#f2d197", fg: "#17140f" }
									: value >= 0.56
										? { bg: "#d9a13d", fg: "#17140f" }
										: value >= 0.4
											? { bg: "#46777d", fg: "var(--f2)" }
											: { bg: "#23485b", fg: "var(--f3)" };

							return (
								<div
									key={symbol}
									title={`${symbol} · ${Math.floor(value * 100)}%`}
									className="flex aspect-square items-center justify-center rounded-[2px] font-mono text-[8px]"
									style={{ backgroundColor: color.bg, color: color.fg }}
								>
									{symbol.split("/")[0] ?? symbol}
								</div>
							);
						})}
					</div>
				)}
			</div>
		</div>
	);
};
