import { createRef } from "react";
import { terminalStore } from "#/collections/terminal";
import type { Measurement } from "#/collections/types";
import {
	paintInspectorMeters,
	mergeInspectorMetrics,
	type MeterParts,
} from "#/components/terminal/inspector-meters";
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
const detailMeters = new Map<string, MeterParts>();

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
	value: Measurement[],
	focusSymbol: string,
) => {
	lastUniverse = value;
	lastFocusSymbol = focusSymbol;

	if (titleRef.current === null) {
		return;
	}

	const source = terminalStore.state.selectedSource;
	const focusRows =
		focusSymbol === ""
			? value.filter((row) => row.source === source)
			: value.filter(
					(row) => row.source === source && row.symbol === focusSymbol,
				);
	const headline =
		source === "hawkes"
			? "conditional_intensity"
			: source === "liquidity"
				? "scarcity_score"
				: source === "toxicity"
					? "touch_quantity"
					: "strength";

	const latest = [...focusRows]
		.reverse()
		.find((row) => row.metrics?.[headline] !== undefined);

	const epoch =
		latest === undefined ? [] : focusRows.filter((row) => row.at === latest.at);

	const active = new Set(
		value.filter((row) => row.source === source).map((row) => row.at),
	).size;

	const total = new Set(value.map((row) => row.at)).size;

	const observedStamp =
		latest === undefined ? Number.NaN : Date.parse(latest.at);

	titleRef.current.textContent = source;

	if (waitingPanelRef.current !== null) {
		waitingPanelRef.current.hidden = epoch.length > 0;
		waitingPanelRef.current.textContent = `waiting for backend ${source} measurement`;
	}

	if (metricsGridRef.current !== null) {
		metricsGridRef.current.hidden = epoch.length === 0;
		if (epoch.length > 0) {
			const entries = mergeInspectorMetrics(epoch, source, focusSymbol);
			paintInspectorMeters(
				metricsGridRef.current,
				detailMeters,
				entries,
				headline,
			);
		}
	}

	if (badgeRef.current !== null) {
		badgeRef.current.textContent =
			latest === undefined
				? "Standby"
				: latest.validity?.state === "invalid"
					? "Fault"
					: latest.validity?.state === "provisional"
						? "Calib"
						: "Healthy";
		badgeRef.current.className = `shrink-0 rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide ${
			latest === undefined
				? "border-(--line2) bg-(--line) text-(--f3)"
				: latest.validity?.state === "invalid"
					? "border-(--down) bg-(--sunken) text-(--down)"
					: latest.validity?.state === "provisional"
						? "border-(--info) bg-(--sunken) text-(--info)"
						: "border-(--up) bg-(--sunken) text-(--up)"
		}`;
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
				})} / ${Math.max(0, (Date.now() - observedStamp) / 1000).toFixed(1)}s`
			: "— / —";
	}

	if (validityRef.current !== null) {
		validityRef.current.textContent =
			latest?.validity?.reason || latest?.validity?.state || "—";
	}

	if (heatmapSectionRef.current !== null) {
		heatmapSectionRef.current.hidden = false;
	}

	if (heatmapTitleRef.current !== null) {
		heatmapTitleRef.current.textContent = `Cross-section · ${headline.replaceAll("_", " ")} heatmap`;
	}

	if (heatmapGridRef.current !== null) {
		const heatmapRows =
			latest === undefined
				? []
				: value.filter(
						(row) =>
							row.source === source &&
							row.at === latest.at &&
							row.metrics?.[headline] !== undefined,
					);

		heatmapGridRef.current.replaceChildren(
			...heatmapRows.map((row) => {
					const cell = document.createElement("div");
					const sample = row.metrics?.[headline];
					const strength = Math.max(
						0,
						Math.min(1, sample?.normalized ?? sample?.raw ?? 0),
					);

					cell.className =
						"flex aspect-square w-8 h-8 items-center justify-center rounded-[2px] font-mono text-[9.5px] font-semibold text-(--f1)";
					cell.style.background = `color-mix(in srgb, var(--acc) ${Math.round(strength * 100)}%, var(--sunken))`;
					cell.textContent = row.symbol.split("/")[0] ?? row.symbol;

					return cell;
				}),
		);
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
			className="mt-4.5 grid grid-cols-2 gap-x-5.5 gap-y-3"
		/>
		<div className="mt-5 grid grid-cols-2 gap-x-5.5 gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
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
			<div ref={heatmapGridRef} className="flex flex-wrap gap-1.5" />
		</div>
	</Flex.Column>
);
