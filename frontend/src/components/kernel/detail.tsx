import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import {
	flattenMeasurementBuffer,
	measurementsStore,
	measurementTickCount,
} from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { buildHeatmapCells } from "#/components/kernel/heatmap";
import {
	kernelCopy,
	kernelStatusMeta,
} from "#/components/terminal/kernel-meta";
import {
	ageText,
	headlineMetric,
	latestByMetric,
	metricLabel,
	orderedEpoch,
	resolveStatus,
	stampOf,
} from "#/components/terminal/measurement-view";
import {
	paintHeatmapGrid,
	paintMetricGrid,
} from "#/components/terminal/metric-paint";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { Flex } from "@/components/ui/flex";

type SignalDetailRefs = {
	badge: HTMLSpanElement | null;
	waitingPanel: HTMLDivElement | null;
	metricsGrid: HTMLDivElement | null;
	activeReadings: HTMLSpanElement | null;
	metricsCount: HTMLSpanElement | null;
	observed: HTMLSpanElement | null;
	validity: HTMLSpanElement | null;
	heatmapSection: HTMLDivElement | null;
	heatmapTitle: HTMLSpanElement | null;
	heatmapGrid: HTMLDivElement | null;
};

const paintSignalDetail = (
	refs: SignalDetailRefs,
	source: string,
	focusSymbol: string,
): void => {
	const measurements = measurementsStore.state.measurements;
	const buffer = measurements[focusSymbol]?.[source];
	const history = flattenMeasurementBuffer(buffer);
	const headline = headlineMetric(source);
	const latest =
		headline === null ? history.at(-1) : latestByMetric(history, headline);
	const epoch = orderedEpoch(history, headline);
	const status = resolveStatus(latest);
	const observedStamp = stampOf(latest?.at);
	const active = Object.values(measurements).reduce(
		(sum, sources) => sum + measurementTickCount(sources[source]),
		0,
	);
	const total = Object.values(measurements).reduce(
		(sum, sources) =>
			sum +
			Object.values(sources).reduce(
				(sourceSum, sourceHistory) =>
					sourceSum + measurementTickCount(sourceHistory),
				0,
			),
		0,
	);
	const heatmap =
		headline === null ? [] : buildHeatmapCells(measurements, source, headline);

	if (refs.waitingPanel !== null) {
		refs.waitingPanel.style.display = epoch.length === 0 ? "" : "none";
	}

	if (refs.metricsGrid !== null) {
		refs.metricsGrid.style.display = epoch.length === 0 ? "none" : "";
		paintMetricGrid(refs.metricsGrid, history, headline);
	}

	if (refs.badge !== null && latest !== undefined) {
		const statusMeta = kernelStatusMeta(status);
		refs.badge.textContent = statusMeta.label;
		refs.badge.style.color = statusMeta.fg;
		refs.badge.style.background = statusMeta.bg;
		refs.badge.style.borderColor = statusMeta.bd;
	}

	if (refs.activeReadings !== null) {
		refs.activeReadings.textContent = `${active.toLocaleString()} / ${total.toLocaleString()}`;
	}

	if (refs.metricsCount !== null) {
		refs.metricsCount.textContent = String(epoch.length);
	}

	if (refs.observed !== null) {
		refs.observed.textContent = Number.isFinite(observedStamp)
			? `${new Date(observedStamp).toLocaleTimeString("en-US", {
					hour12: false,
				})} / ${ageText(observedStamp)}`
			: "— / —";
	}

	if (refs.validity !== null) {
		refs.validity.textContent =
			latest?.validity?.reason || latest?.validity?.state || "—";
	}

	if (refs.heatmapSection !== null) {
		refs.heatmapSection.style.display = headline === null ? "none" : "";
	}

	if (refs.heatmapTitle !== null && headline !== null) {
		refs.heatmapTitle.textContent = `Cross-section · ${metricLabel(headline)} heatmap`;
	}

	if (refs.heatmapGrid !== null && headline !== null) {
		paintHeatmapGrid(refs.heatmapGrid, heatmap);
	}
};

/*
SignalDetail renders a static signal-insight shell and paints every live
measurement readout directly from the measurement store.
*/
export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const badgeRef = useRef<HTMLSpanElement>(null);
	const waitingPanelRef = useRef<HTMLDivElement>(null);
	const metricsGridRef = useRef<HTMLDivElement>(null);
	const activeReadingsRef = useRef<HTMLSpanElement>(null);
	const metricsCountRef = useRef<HTMLSpanElement>(null);
	const observedRef = useRef<HTMLSpanElement>(null);
	const validityRef = useRef<HTMLSpanElement>(null);
	const heatmapSectionRef = useRef<HTMLDivElement>(null);
	const heatmapTitleRef = useRef<HTMLDivElement>(null);
	const heatmapGridRef = useRef<HTMLDivElement>(null);
	const copy = kernelCopy(selectedSource, selectedSource);

	useDirectStorePaint(
		() =>
			paintSignalDetail(
				{
					badge: badgeRef.current,
					waitingPanel: waitingPanelRef.current,
					metricsGrid: metricsGridRef.current,
					activeReadings: activeReadingsRef.current,
					metricsCount: metricsCountRef.current,
					observed: observedRef.current,
					validity: validityRef.current,
					heatmapSection: heatmapSectionRef.current,
					heatmapTitle: heatmapTitleRef.current,
					heatmapGrid: heatmapGridRef.current,
				},
				selectedSource,
				focusSymbol,
			),
		[measurementsStore],
		[selectedSource, focusSymbol],
	);

	return (
		<Flex.Column className="min-h-0 overflow-auto px-5 py-[18px]">
			<Flex.Row className="items-start justify-between gap-3">
				<Flex.Column>
					<Flex className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
						{copy.name}
					</Flex>
					<Flex className="mt-1 font-mono text-[11px] text-(--f3)">
						{copy.sub}
					</Flex>
				</Flex.Column>
				<span
					ref={badgeRef}
					className="shrink-0 rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
				/>
			</Flex.Row>
			<Flex className="mt-3.5 max-w-[560px] font-serif text-[15px] text-(--f2) leading-[1.55]">
				{copy.blurb}
			</Flex>
			<div
				ref={waitingPanelRef}
				className="mt-[18px] rounded-[3px] border border-(--line) bg-(--sunken) px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
			>
				waiting for backend {selectedSource} measurement
			</div>
			<div
				ref={metricsGridRef}
				className="mt-[18px] grid gap-x-[22px] gap-y-3"
				style={{ gridTemplateColumns: "repeat(2, minmax(0, 1fr))" }}
			/>
			<div
				className="mt-5 grid gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs"
				style={{ gridTemplateColumns: "repeat(2, minmax(0, 1fr))" }}
			>
				<div className="flex justify-between">
					<span className="text-(--f3)">Active readings</span>
					<span ref={activeReadingsRef} className="text-(--f1)" />
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Metrics this tick</span>
					<span ref={metricsCountRef} className="text-(--f1)" />
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Observed</span>
					<span ref={observedRef} className="text-(--f1)" />
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Validity</span>
					<span ref={validityRef} className="text-(--f1)" />
				</div>
			</div>
			<div ref={heatmapSectionRef} className="mt-[18px]">
				<div
					ref={heatmapTitleRef}
					className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]"
				/>
				<div
					ref={heatmapGridRef}
					className="grid gap-[3px]"
					style={{ gridTemplateColumns: "repeat(12, minmax(0, 1fr))" }}
				/>
			</div>
		</Flex.Column>
	);
};
