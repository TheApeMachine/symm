import { memo, useRef } from "react";
import {
	headlineSeriesFromBuffer,
	latestMeasurementReadings,
	latestPublishedStamp,
	measurementsStore,
} from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	kernelStatusVariant,
} from "#/components/terminal/kernel-meta";
import { sourceHasUniverseFrames } from "#/components/terminal/measurement-sources";
import {
	ageText,
	formatRaw,
	headlineMetric,
	headlineReading,
	metricLabel,
	percentOf,
	resolveKernelStatus,
	stampOf,
} from "#/components/terminal/measurement-view";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { Badge, badgeVariants } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type KernelListRowRefs = {
	button: HTMLButtonElement | null;
	sparkArea: SVGPolylineElement | null;
	sparkLine: SVGPolylineElement | null;
	barFill: HTMLDivElement | null;
	readout: HTMLSpanElement | null;
	age: HTMLSpanElement | null;
	badge: HTMLSpanElement | null;
	statusDot: HTMLSpanElement | null;
	compactReadout: HTMLDivElement | null;
};

const paintKernelListRow = (
	refs: KernelListRowRefs,
	source: string,
	focusSymbol: string,
): void => {
	const buffer = measurementsStore.state.measurements[focusSymbol]?.[source];
	const epoch = latestMeasurementReadings(buffer);
	const headline = headlineMetric(source);
	const latest = headlineReading(epoch, source);
	const activeMetric = latest?.metric ?? headline;
	const status = resolveKernelStatus(
		latest,
		sourceHasUniverseFrames(measurementsStore.state, source),
	);
	const statusMeta = kernelStatusMeta(status);
	const spark = kernelSparkPaths(
		activeMetric === null || activeMetric === undefined
			? []
			: headlineSeriesFromBuffer(buffer, activeMetric),
		status,
	);
	const percent = latest === undefined ? 0 : percentOf(latest);
	const readout =
		headline === null
			? `${epoch.length} readings`
			: latest === undefined || activeMetric === undefined
				? "—"
				: `${metricLabel(activeMetric)} ${formatRaw(latest)}`;
	const { inspectorSource, selectedSource } = terminalStore.state;
	const active = inspectorSource === source || selectedSource === source;

	if (refs.button !== null) {
		refs.button.style.borderLeftColor = active ? "var(--acc)" : "transparent";
		refs.button.style.background = active ? "var(--raised)" : "transparent";
		refs.button.setAttribute("aria-pressed", active ? "true" : "false");
	}

	if (refs.sparkArea !== null) {
		refs.sparkArea.setAttribute("points", spark.area);
		refs.sparkArea.setAttribute("fill", spark.fill);
	}

	if (refs.sparkLine !== null) {
		refs.sparkLine.setAttribute("points", spark.spark);
		refs.sparkLine.setAttribute("stroke", spark.line);
	}

	if (refs.barFill !== null) {
		refs.barFill.style.width = `${percent}%`;
		refs.barFill.style.background = spark.active
			? "var(--warning)"
			: "var(--info)";
	}

	if (refs.readout !== null) {
		refs.readout.textContent = readout;
	}

	if (refs.age !== null) {
		refs.age.textContent = ageText(
			stampOf(latestPublishedStamp(buffer) ?? latest?.at),
		);
	}

	if (refs.badge !== null) {
		refs.badge.textContent = statusMeta.label;
		refs.badge.className = cn(
			badgeVariants({
				variant: kernelStatusVariant(status),
				size: "xs",
			}),
			"shrink-0 font-mono",
		);
	}

	if (refs.statusDot !== null) {
		refs.statusDot.style.backgroundColor = statusMeta.fg;
	}

	if (refs.compactReadout !== null) {
		refs.compactReadout.textContent =
			latest === undefined ? statusMeta.label : readout;
	}
};

/*
KernelListRow paints one signal kernel row directly from measurement store
snapshots so websocket cadence never re-renders the surrounding React tree.
*/
export const KernelListRow = memo(
	({
		source,
		focusSymbol,
		compact = false,
	}: {
		source: string;
		focusSymbol: string;
		compact?: boolean;
	}) => {
		const buttonRef = useRef<HTMLButtonElement>(null);
		const sparkAreaRef = useRef<SVGPolylineElement>(null);
		const sparkLineRef = useRef<SVGPolylineElement>(null);
		const barFillRef = useRef<HTMLDivElement>(null);
		const readoutRef = useRef<HTMLSpanElement>(null);
		const ageRef = useRef<HTMLSpanElement>(null);
		const badgeRef = useRef<HTMLSpanElement>(null);
		const statusDotRef = useRef<HTMLSpanElement>(null);
		const compactReadoutRef = useRef<HTMLDivElement>(null);
		const copy = kernelCopy(source, source);
		const { inspectSource, selectSource } = terminalStore.actions;

		useDirectStorePaint(
			() =>
				paintKernelListRow(
					{
						button: buttonRef.current,
						sparkArea: sparkAreaRef.current,
						sparkLine: sparkLineRef.current,
						barFill: barFillRef.current,
						readout: readoutRef.current,
						age: ageRef.current,
						badge: badgeRef.current,
						statusDot: statusDotRef.current,
						compactReadout: compactReadoutRef.current,
					},
					source,
					focusSymbol,
				),
			[measurementsStore, terminalStore],
			[source, focusSymbol, compact],
		);

		return (
			<button
				ref={buttonRef}
				type="button"
				onClick={() => (compact ? selectSource(source) : inspectSource(source))}
				className="block w-full cursor-pointer border-(--line) border-b border-l-2 px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
				style={{
					borderLeftColor: "transparent",
					background: "transparent",
				}}
			>
				<div className="flex items-center justify-between gap-2">
					<span
						className={`truncate font-semibold text-(--f1) ${
							compact ? "text-xs" : "text-[12.5px]"
						}`}
					>
						{compact ? copy.name : source}
					</span>

					{compact ? (
						<span
							ref={statusDotRef}
							className="size-[7px] shrink-0 rounded-full"
						/>
					) : (
						<Badge
							ref={badgeRef}
							label=""
							size="xs"
							className="shrink-0 font-mono"
						/>
					)}
				</div>

				{compact ? null : (
					<>
						<svg
							viewBox="0 0 150 30"
							preserveAspectRatio="none"
							className="mt-1.5 block h-[26px] w-full"
						>
							<title>Signal sparkline</title>
							<polyline
								ref={sparkAreaRef}
								fill="color-mix(in srgb, var(--info) 12%, transparent)"
								stroke="none"
							/>
							<polyline
								ref={sparkLineRef}
								fill="none"
								stroke="var(--info)"
								strokeWidth="1.4"
								vectorEffect="non-scaling-stroke"
							/>
						</svg>

						<div className="mt-1.5 flex items-center gap-2">
							{headlineMetric(source) === null ? null : (
								<div className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
									<div
										ref={barFillRef}
										className="h-full transition-[width] duration-500 ease-out"
										style={{ width: "0%" }}
									/>
								</div>
							)}

							<span
								ref={readoutRef}
								className="flex-1 truncate text-right font-mono text-[10px] text-(--f2)"
							/>

							<span
								ref={ageRef}
								className="w-[46px] shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
							/>
						</div>
					</>
				)}

				{compact ? (
					<div
						ref={compactReadoutRef}
						className="mt-1 truncate font-mono text-[9px] text-(--f4)"
					/>
				) : null}
			</button>
		);
	},
);
