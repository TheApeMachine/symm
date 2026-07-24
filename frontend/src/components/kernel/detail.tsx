import { createRef } from "react";
import { terminalStore } from "#/collections/terminal";
import type { Measurement } from "#/collections/types";
import { buildHeatmapCells } from "#/components/kernel/heatmap";
import { kernelStatusMeta } from "#/components/terminal/kernel-meta";
import {
	ageText,
	headlineMetric,
	latestByMetric,
	metricLabel,
	orderedEpoch,
	resolveStatus,
	rowsFromBuffer,
	stampOf,
} from "#/components/terminal/measurement-view";
import {
	paintHeatmapGrid,
	paintMetricGrid,
} from "#/components/terminal/metric-paint";
import { frameRows } from "#/providers/frame-history";
import { Flex } from "@/components/ui/flex";

const titleRef = createRef<HTMLSpanElement>();
const waitingPanelRef = createRef<HTMLDivElement>();
const metricsGridRef = createRef<HTMLDivElement>();
const activeReadingsRef = createRef<HTMLSpanElement>();
const metricsCountRef = createRef<HTMLSpanElement>();
const observedRef = createRef<HTMLSpanElement>();
const validityRef = createRef<HTMLSpanElement>();
const heatmapSectionRef = createRef<HTMLDivElement>();
const heatmapTitleRef = createRef<HTMLDivElement>();
const heatmapGridRef = createRef<HTMLDivElement>();
const badgeRef = createRef<HTMLSpanElement>();

let lastUniverse: Measurement[] = [];
let lastFocusSymbol = "";

const measurementTickCount = (rows: Measurement[]): number => {
	if (rows.length === 0) {
		return 0;
	}

	return new Set(rows.map((row) => row.at)).size;
};

/*
repaintSignalDetail paints SignalDetail from retained measurement history after
a source click without requiring a store subscription.
*/
export const repaintSignalDetail = () => {
	paintSignalDetailMeasurements(lastUniverse, lastFocusSymbol);
};

/*
paintSignalDetailMeasurements paints SignalDetail from ordered measurement
history and the current focusSymbol.
*/
export const paintSignalDetailMeasurements = (
	value: unknown,
	focusSymbol: string,
) => {
	lastUniverse = frameRows<Measurement>(value);
	lastFocusSymbol = focusSymbol;

	if (titleRef.current === null) {
		return;
	}

	const source = terminalStore.state.selectedSource;
	const universe = lastUniverse;
	const focusRows =
		focusSymbol === ""
			? universe
			: universe.filter((row) => row.symbol === focusSymbol);
	const history = rowsFromBuffer(
		focusRows.filter((row) => row.source === source),
	);
	const headline = headlineMetric(source);
	const latest =
		headline === null ? history.at(-1) : latestByMetric(history, headline);
	const epoch = orderedEpoch(history, headline);
	const status = resolveStatus(latest);
	const observedStamp = stampOf(latest?.at);
	const active = measurementTickCount(
		universe.filter((row) => row.source === source),
	);
	const total = measurementTickCount(universe);
	const heatmap =
		headline === null ? [] : buildHeatmapCells(universe, source, headline);

	if (titleRef.current !== null) {
		titleRef.current.textContent = source;
	}

	if (waitingPanelRef.current !== null) {
		waitingPanelRef.current.style.display = epoch.length === 0 ? "" : "none";
		waitingPanelRef.current.textContent = `waiting for backend ${source} measurement`;
	}

	if (metricsGridRef.current !== null) {
		metricsGridRef.current.style.display = epoch.length === 0 ? "none" : "";
		paintMetricGrid(metricsGridRef.current, history, headline);
	}

	if (badgeRef.current !== null) {
		const statusMeta = kernelStatusMeta(status);
		badgeRef.current.textContent = statusMeta.label;
		badgeRef.current.style.color = statusMeta.fg;
		badgeRef.current.style.background = statusMeta.bg;
		badgeRef.current.style.borderColor = statusMeta.bd;
	}

	if (activeReadingsRef.current !== null) {
		activeReadingsRef.current.textContent = `${active.toLocaleString()} / ${total.toLocaleString()}`;
	}

	if (metricsCountRef.current !== null) {
		metricsCountRef.current.textContent = String(epoch.length);
	}

	if (observedRef.current !== null) {
		observedRef.current.textContent = Number.isFinite(observedStamp)
			? `${new Date(observedStamp).toLocaleTimeString("en-US", {
					hour12: false,
				})} / ${ageText(observedStamp)}`
			: "— / —";
	}

	if (validityRef.current !== null) {
		validityRef.current.textContent =
			latest?.validity?.reason || latest?.validity?.state || "—";
	}

	if (heatmapSectionRef.current !== null) {
		heatmapSectionRef.current.style.display = headline === null ? "none" : "";
	}

	if (heatmapTitleRef.current !== null && headline !== null) {
		heatmapTitleRef.current.textContent = `Cross-section · ${metricLabel(headline)} heatmap`;
	}

	if (heatmapGridRef.current !== null && headline !== null) {
		paintHeatmapGrid(heatmapGridRef.current, heatmap);
	}
};

/*
SignalDetail is the static signal-insight shell. DRAW paints via
paintSignalDetailMeasurements(value, focusSymbol).
*/
export const SignalDetail = () => (
	<Flex.Column className="min-h-0 overflow-auto px-5 py-4.5">
		<Flex.Row className="items-start justify-between gap-3">
			<span
				ref={titleRef}
				className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]"
			/>
			<span
				ref={badgeRef}
				className="shrink-0 rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
			/>
		</Flex.Row>
		<div
			ref={waitingPanelRef}
			className="mt-4.5 rounded-[3px] border border-(--line) bg-(--sunken) px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
		/>
		<div
			ref={metricsGridRef}
			className="mt-4.5 grid gap-x-5.5 gap-y-3"
			style={{ gridTemplateColumns: "repeat(2, minmax(0, 1fr))" }}
		/>
		<div
			className="mt-5 grid gap-x-5.5 gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs"
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
		<div ref={heatmapSectionRef} className="mt-4.5">
			<div
				ref={heatmapTitleRef}
				className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]"
			/>
			<div
				ref={heatmapGridRef}
				className="grid gap-0.75"
				style={{ gridTemplateColumns: "repeat(12, minmax(0, 1fr))" }}
			/>
		</div>
	</Flex.Column>
);
