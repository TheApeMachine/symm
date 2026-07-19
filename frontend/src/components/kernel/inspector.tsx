import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import {
	flattenMeasurementBuffer,
	latestMeasurementReadings,
	measurementTickCount,
	measurementsForSource,
} from "#/collections/measurements";
import type { Measurement } from "#/collections/types";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
} from "#/components/terminal/kernel-meta";
import {
	headlineMetric,
	latestByMetric,
	metricLabel,
	resolveStatus,
} from "#/components/terminal/measurement-view";
import { paintMetricGrid } from "#/components/terminal/metric-paint";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { requireSampleSize } from "#/lib/domain";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
import { panelVariants } from "@/components/ui/panel";

type InspectorPaintRefs = {
	badge: HTMLSpanElement | null;
	seriesLabel: HTMLSpanElement | null;
	sparkLine: SVGPolylineElement | null;
	metricsGrid: HTMLDivElement | null;
	activeSymbol: HTMLDivElement | null;
	observed: HTMLDivElement | null;
};

const paintInspector = (
	refs: InspectorPaintRefs,
	source: string,
	history: Measurement[],
): void => {
	const headline = headlineMetric(source);
	const latestEpoch = latestMeasurementReadings(history);
	const latest =
		headline === null
			? latestEpoch.at(-1)
			: latestByMetric(latestEpoch, headline);

	if (latest === undefined) {
		return;
	}

	const status = resolveStatus(latest);
	const statusMeta = kernelStatusMeta(status);
	const seriesSource = headline ?? history.at(-1)?.metric;
	const series =
		seriesSource === undefined
			? []
			: history.flatMap((measurement) => {
					if (measurement.metric !== seriesSource) {
						return [];
					}

					return Number.isFinite(measurement.raw) ? [measurement] : [];
				});
	const rawValues = series.map((measurement) => measurement.raw);
	const minimum = rawValues.length > 0 ? Math.min(...rawValues) : 0;
	const maximum = rawValues.length > 0 ? Math.max(...rawValues) : 0;
	const values = series.map((measurement) => {
		if (typeof measurement.normalized === "number") {
			return Math.max(0, Math.min(1, measurement.normalized));
		}

		if (maximum > minimum) {
			return (measurement.raw - minimum) / (maximum - minimum);
		}

		return measurement.raw > 0 ? 0.5 : 0;
	});
	const width = 150;
	const baseline = 29;
	const scale = 26;
	const spark = values.slice(-40);
	const points =
		spark.length < 2
			? ""
			: (() => {
					requireSampleSize(spark.length, 2, "inspector sparkline");
					const span = spark.length - 1;

					return spark
						.map(
							(value, index) =>
								`${((index / span) * width).toFixed(1)},${(
									baseline - Math.max(0, Math.min(1, value)) * scale
								).toFixed(1)}`,
						)
						.join(" ");
				})();
	const observed = latest.at
		? new Date(latest.at).toLocaleTimeString("en-US", { hour12: false })
		: "—";

	if (refs.badge !== null) {
		refs.badge.textContent = statusMeta.label;
		refs.badge.style.color = statusMeta.fg;
		refs.badge.style.background = statusMeta.bg;
		refs.badge.style.borderColor = statusMeta.bd;
	}

	if (refs.seriesLabel !== null) {
		refs.seriesLabel.textContent = metricLabel(seriesSource);
	}

	if (refs.sparkLine !== null) {
		refs.sparkLine.setAttribute("points", points);
	}

	if (refs.metricsGrid !== null) {
		paintMetricGrid(refs.metricsGrid, history, headline);
	}

	if (refs.activeSymbol !== null) {
		refs.activeSymbol.textContent = `active ${latest.symbol}`;
	}

	if (refs.observed !== null) {
		refs.observed.textContent = `observed ${observed} · ${measurementTickCount(history)} samples`;
	}
};

/*
KernelInspectorPanel renders a static inspector shell and paints every
high-frequency readout directly from the measurement store.
*/
const KernelInspectorPanel = ({
	source,
	focusSymbol,
}: {
	source: string;
	focusSymbol: string;
}) => {
	const badgeRef = useRef<HTMLSpanElement>(null);
	const seriesLabelRef = useRef<HTMLSpanElement>(null);
	const sparkLineRef = useRef<SVGPolylineElement>(null);
	const metricsGridRef = useRef<HTMLDivElement>(null);
	const activeSymbolRef = useRef<HTMLDivElement>(null);
	const observedRef = useRef<HTMLDivElement>(null);
	const online = useSelector(appStore, (state) => state.online);
	const { closeInspect, inspectSource } = terminalStore.actions;
	const copy = kernelCopy(source, source);

	useDirectStorePaint(
		getWorker(),
		[{ store: "measurements", key: focusSymbol }],
		(buffers) => {
			paintInspector(
				{
					badge: badgeRef.current,
					seriesLabel: seriesLabelRef.current,
					sparkLine: sparkLineRef.current,
					metricsGrid: metricsGridRef.current,
					activeSymbol: activeSymbolRef.current,
					observed: observedRef.current,
				},
				source,
				flattenMeasurementBuffer(
					measurementsForSource(
						(buffers[`measurements:${focusSymbol}`] ?? []) as Measurement[],
						source,
					),
				),
			);
		},
		[online, source, focusSymbol],
	);

	return (
		<div className="absolute inset-y-0 left-[282px] right-[332px] z-9 flex animate-[symFade_0.18s_ease] flex-col bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)] p-8 backdrop-blur-[3px]">
			<button
				type="button"
				aria-label="Close kernel inspector"
				className="absolute inset-0"
				onClick={closeInspect}
			/>
			<div className="pointer-events-none relative z-10 flex min-h-0 flex-1 items-center justify-center">
				<div className="pointer-events-auto flex max-h-full w-full max-w-[452px] flex-col overflow-hidden rounded-[6px] border border-(--line2) bg-(--surface) shadow-[0_22px_54px_-14px_rgba(0,0,0,0.72)]">
					<div className="flex shrink-0 items-start justify-between gap-2.5 border-(--line) border-b px-4 pt-3.5 pb-[13px]">
						<div className="min-w-0">
							<div className="flex items-center gap-2">
								<span className="font-serif font-semibold text-[19px] text-(--f1) leading-[1.1]">
									{copy.name}
								</span>
								<span
									ref={badgeRef}
									className="shrink-0 rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
								/>
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
					<p className="mx-4 mt-3.5 shrink-0 font-serif text-[14px] text-(--f2) leading-[1.56]">
						{copy.blurb}
					</p>
					<div className="mx-4 mt-3.5 shrink-0">
						<div className="mb-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4) uppercase tracking-widest">
							<span>signal history</span>
							<span ref={seriesLabelRef} />
						</div>
						<svg
							viewBox="0 0 150 30"
							preserveAspectRatio="none"
							className={cn(
								panelVariants({ size: "bare" }),
								"block h-[52px] w-full",
							)}
						>
							<title>Signal history</title>
							<polyline
								ref={sparkLineRef}
								fill="none"
								stroke="var(--acc)"
								strokeWidth="1.5"
								vectorEffect="non-scaling-stroke"
							/>
						</svg>
					</div>
					<div
						ref={metricsGridRef}
						className="grid min-h-0 flex-1 grid-cols-2 content-start gap-x-3 gap-y-2.5 overflow-y-auto px-4 pt-3.5 pb-0.5"
					/>
					<div className="mt-[11px] flex shrink-0 items-center justify-between gap-3 border-(--line) border-t bg-(--sunken) px-4 py-3.5">
						<div className="min-w-0 font-mono text-[9.5px] text-(--f4) leading-[1.55]">
							<div ref={activeSymbolRef} />
							<div ref={observedRef} />
						</div>
						<button
							type="button"
							onClick={() => inspectSource(source)}
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

/*
KernelInspector opens a vertically scaled detail modal for one signal source.
Only open/close state uses React; measurement readouts bypass reconciliation.
*/
export const KernelInspector = () => {
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	if (inspectorSource === null) {
		return null;
	}

	return (
		<KernelInspectorPanel source={inspectorSource} focusSymbol={focusSymbol} />
	);
};
