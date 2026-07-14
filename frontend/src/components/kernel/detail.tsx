import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { StatusBadge } from "#/components/dashboard/status-badge";
import {
	kernelCopy,
	kernelStatusMeta,
} from "#/components/terminal/kernel-meta";
import {
	ageText,
	formatRaw,
	headlineMetric,
	latestByMetric,
	latestEpoch,
	metricLabel,
	percentOf,
	resolveStatus,
	sideLabel,
	stampOf,
} from "#/components/terminal/measurement-view";
import { InspectorMeter } from "./meter";

const clampPercent = (value: number) => Math.max(0, Math.min(100, value * 100));

export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const measurements = useSelector(measurementsStore, (state) => state);
	const source = selectedSource;
	const history =
		measurements.measurements[focusSymbol]?.[source]?.values() ?? [];
	const headline = headlineMetric(source);
	const latest =
		headline === null ? history.at(-1) : latestByMetric(history, headline);
	const epoch = latestEpoch(history);
	const status = resolveStatus(latest);
	const copy = kernelCopy(selectedSource, selectedSource);
	const statusMeta = kernelStatusMeta(status);
	const observedStamp = stampOf(latest?.at);
	const active = Object.values(measurements.measurements).reduce(
		(sum, sources) => sum + (sources[source]?.values().length ?? 0),
		0,
	);
	const total = Object.values(measurements.measurements).reduce(
		(sum, sources) =>
			sum +
			Object.values(sources).reduce(
				(sourceSum, sourceHistory) => sourceSum + sourceHistory.values().length,
				0,
			),
		0,
	);
	const heatmap =
		headline === null
			? []
			: Object.entries(measurements.measurements).flatMap(
					([symbol, sources]) => {
						const frame = latestByMetric(
							sources[source]?.values() ?? [],
							headline,
						);

						return frame === undefined
							? []
							: [{ symbol, value: percentOf(frame) / 100 }];
					},
				);

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
				<StatusBadge label={statusMeta.label} tone={statusMeta.fg} />
			</div>
			<p className="mt-3.5 max-w-[560px] font-serif text-[15px] text-(--f2) leading-[1.55]">
				{copy.blurb}
			</p>
			{epoch.length === 0 ? (
				<div className="mt-[18px] rounded border border-(--line) bg-(--sunken) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
					waiting for backend {selectedSource} measurement
				</div>
			) : null}
			{epoch.length === 0 ? null : (
				<div className="mt-[18px] grid grid-cols-2 gap-x-[22px] gap-y-3">
					{epoch.map((measurement) => (
						<InspectorMeter
							key={`${measurement.metric}:${measurement.side ?? ""}`}
							label={[
								metricLabel(measurement.metric),
								sideLabel(measurement.side),
							]
								.filter(Boolean)
								.join(" · ")}
							value={formatRaw(measurement)}
							percent={percentOf(measurement)}
							color={
								measurement.metric === headline ? "var(--acc)" : "var(--info)"
							}
						/>
					))}
				</div>
			)}
			<div className="mt-5 grid grid-cols-2 gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
				<div className="flex justify-between">
					<span className="text-(--f3)">Active readings</span>
					<span className="text-(--f1)">
						{active.toLocaleString()} / {total.toLocaleString()}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Metrics this tick</span>
					<span className="text-(--f1)">{epoch.length}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Observed</span>
					<span className="text-(--f1)">
						{Number.isFinite(observedStamp)
							? `${new Date(observedStamp).toLocaleTimeString("en-US", {
									hour12: false,
								})} / ${ageText(observedStamp)}`
							: "— / —"}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Validity</span>
					<span className="text-(--f1)">
						{latest?.validity?.reason || latest?.validity?.state || "—"}
					</span>
				</div>
			</div>
			{headline === null ? null : (
				<div className="mt-[18px]">
					<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Cross-section · {metricLabel(headline)} heatmap
					</div>
					<div className="grid grid-cols-12 gap-[3px]">
						{heatmap.map((cell) => {
							const percent = Math.round(clampPercent(cell.value));
							const label = cell.symbol.split("/")[0] ?? cell.symbol;

							return (
								<div
									key={cell.symbol}
									data-symbol={cell.symbol}
									title={`${cell.symbol} · ${percent}%`}
									className="flex aspect-square cursor-pointer items-center justify-center rounded-[2px] font-mono text-[8px]"
									style={{
										background: `color-mix(in srgb, var(--acc) ${percent}%, var(--sunken))`,
										color: percent > 62 ? "#14110f" : "var(--f3)",
									}}
								>
									{label}
								</div>
							);
						})}
					</div>
				</div>
			)}
		</div>
	);
};
