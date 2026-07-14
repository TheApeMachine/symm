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
	formatRaw,
	headlineMetric,
	latestByMetric,
	latestEpoch,
	metricLabel,
	percentOf,
	resolveStatus,
	sideLabel,
} from "#/components/terminal/measurement-view";
import { InspectorMeter } from "./meter";

export const KernelInspector = () => {
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { closeInspect, inspectSource } = terminalStore.actions;
	const source = inspectorSource ?? "";
	const history = useSelector(measurementsStore, (state) => {
		if (source === "") {
			return [];
		}

		return state.measurements[focusSymbol]?.[source]?.values() ?? [];
	});
	const headline = headlineMetric(source);
	const latest =
		headline === null ? history.at(-1) : latestByMetric(history, headline);

	if (inspectorSource === null || latest === undefined) {
		return null;
	}

	const epoch = latestEpoch(history);
	const status = resolveStatus(latest);
	const copy = kernelCopy(source, source);
	const statusMeta = kernelStatusMeta(status);
	const width = 150;
	const baseline = 29;
	const scale = 26;
	const seriesSource = headline ?? epoch[0]?.metric;
	const values = seriesSource
		? history.flatMap((measurement) => {
				if (
					measurement.metric !== seriesSource ||
					(measurement.side ?? "") !== ""
				) {
					return [];
				}

				return Number.isFinite(measurement.raw) ? [measurement.raw] : [];
			})
		: [];
	const points = values
		.slice(-40)
		.map(
			(value, index, series) =>
				`${((index / Math.max(series.length - 1, 1)) * width).toFixed(
					1,
				)},${(baseline - Math.max(0, Math.min(1, value)) * scale).toFixed(1)}`,
		)
		.join(" ");
	const observed = latest.at
		? new Date(latest.at).toLocaleTimeString("en-US", { hour12: false })
		: "—";

	return (
		<div className="absolute inset-y-0 left-[282px] right-[332px] z-9 animate-[symFade_0.18s_ease] bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)] p-8 backdrop-blur-[3px]">
			<button
				type="button"
				aria-label="Close kernel inspector"
				className="absolute inset-0"
				onClick={closeInspect}
			/>
			<div className="pointer-events-none relative z-10 flex w-full items-start justify-center">
				<div className="pointer-events-auto w-full max-w-[452px] overflow-hidden rounded-[6px] border border-(--line2) bg-(--surface) shadow-[0_22px_54px_-14px_rgba(0,0,0,0.72)]">
					<div className="flex items-start justify-between gap-2.5 border-(--line) border-b px-4 pt-3.5 pb-[13px]">
						<div className="min-w-0">
							<div className="flex items-center gap-2">
								<span className="font-serif font-semibold text-[19px] text-(--f1) leading-[1.1]">
									{copy.name}
								</span>
								<StatusBadge label={statusMeta.label} tone={statusMeta.fg} />
							</div>
							<div className="mt-1 font-mono text-[10px] text-(--f4)">
								{copy.sub}
							</div>
						</div>
						<button
							type="button"
							onClick={closeInspect}
							className="flex size-[25px] shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
						>
							<svg
								width="13"
								height="13"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								strokeWidth="2"
								aria-hidden="true"
							>
								<path d="M6 6l12 12M18 6L6 18" />
							</svg>
						</button>
					</div>
					<p className="mx-4 mt-3.5 font-serif text-[14px] text-(--f2) leading-[1.56]">
						{copy.blurb}
					</p>
					<div className="mx-4 mt-3.5">
						<div className="mb-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4) uppercase tracking-widest">
							<span>signal history</span>
							<span>{metricLabel(seriesSource)}</span>
						</div>
						<svg
							viewBox="0 0 150 30"
							preserveAspectRatio="none"
							className="block h-[52px] w-full rounded-[3px] border border-(--line) bg-(--sunken)"
						>
							<title>Signal history</title>
							<polyline
								points={points}
								fill="none"
								stroke="var(--acc)"
								strokeWidth="1.5"
								vectorEffect="non-scaling-stroke"
							/>
						</svg>
					</div>
					<div className="flex flex-col gap-2.5 px-4 pt-3.5 pb-0.5">
						{epoch.slice(0, 4).map((measurement) => (
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
					<div className="mt-[11px] flex items-center justify-between gap-3 border-(--line) border-t bg-(--sunken) px-4 py-3.5">
						<div className="min-w-0 font-mono text-[9.5px] text-(--f4) leading-[1.55]">
							<div>active {latest.symbol}</div>
							<div>
								observed {observed} · {history.length} samples
							</div>
						</div>
						<button
							type="button"
							onClick={() => inspectSource(inspectorSource)}
							className="shrink-0 cursor-pointer rounded-[3px] border border-[color-mix(in_srgb,var(--acc)_45%,transparent)] bg-[color-mix(in_srgb,var(--acc)_12%,transparent)] px-3 py-2 font-semibold text-[11px] text-(--acc) whitespace-nowrap hover:bg-[color-mix(in_srgb,var(--acc)_22%,transparent)]"
						>
							Open in signal insight →
						</button>
					</div>
				</div>
			</div>
		</div>
	);
};
