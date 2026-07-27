import { createRef } from "react";
import type { DiagnosticsFrame, Measurement } from "#/collections/types";
import { paintInlineMeter } from "#/components/terminal/metric-paint";
import { Panel } from "@/components/ui/panel";

export type CrossSectionReadout = {
	leader: string;
	leadershipThresholdPercent: number;
	breadthPercent: number;
	symbolCount: number;
	medianVolume: number;
	medianQuoteNotional: number;
	medianExecutableDepth: number;
};

const finiteNumber = (value: unknown): number =>
	typeof value === "number" && Number.isFinite(value) ? value : 0;

const asRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

const stringArray = (value: unknown): string[] =>
	Array.isArray(value)
		? value.filter((entry): entry is string => typeof entry === "string")
		: [];

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter(
				(entry): entry is number =>
					typeof entry === "number" && Number.isFinite(entry),
			)
		: [];

const metricRaw = (measurement: Measurement, metric: string): number => {
	const sample =
		measurement.metrics?.[metric] ?? measurement.metrics?.[`${metric}/`];

	return finiteNumber(sample?.raw);
};

const metricMagnitude = (measurement: Measurement): number =>
	Object.values(measurement.metrics ?? {}).reduce(
		(max, sample) => Math.max(max, Math.abs(finiteNumber(sample?.raw))),
		0,
	);

const symbolMetricsFromMeasurements = (
	measurements: Measurement[],
): Array<{
	symbol: string;
	volume: number;
	quoteNotional: number;
	executableDepth: number;
	magnitude: number;
}> => {
	const latest = new Map<string, Map<string, Measurement>>();

	for (const measurement of measurements) {
		if (typeof measurement.symbol !== "string" || measurement.symbol === "") {
			continue;
		}

		const bucket =
			latest.get(measurement.symbol) ?? new Map<string, Measurement>();
		bucket.set(measurement.source, measurement);
		latest.set(measurement.symbol, bucket);
	}

	return [...latest.entries()].map(([symbol, bucket]) => {
		const rows = [...bucket.values()];

		return {
			symbol,
			volume: 0,
			quoteNotional: Math.max(
				...rows.map((row) => metricRaw(row, "reported_volume_notional")),
			),
			executableDepth: Math.max(
				...rows.map((row) => metricRaw(row, "executable_touch_depth")),
			),
			magnitude: Math.max(...rows.map(metricMagnitude)),
		};
	});
};

/*
symbolMetricsFromFrame reads backend CrossSection.Metrics rows and falls back to
legacy flat arrays when an older diagnostics frame is still in flight.
*/
const symbolMetricsFromFrame = (
	frame: Record<string, unknown> | null,
): Array<{
	symbol: string;
	volume: number;
	quoteNotional: number;
	executableDepth: number;
}> => {
	const metrics = frame?.metrics;

	if (Array.isArray(metrics)) {
		return metrics.flatMap((entry) => {
			const row = asRecord(entry);

			if (row === null) {
				return [];
			}

			const symbol = typeof row.symbol === "string" ? row.symbol.trim() : "";

			if (symbol === "") {
				return [];
			}

			return [
				{
					symbol,
					volume: finiteNumber(row.volume),
					quoteNotional: finiteNumber(row.quoteNotional),
					executableDepth: finiteNumber(row.executableDepth),
				},
			];
		});
	}

	const symbols = stringArray(frame?.symbols);
	const volumes = numberArray(frame?.volumes);
	const quoteNotionals = numberArray(frame?.quoteNotionals);
	const executableDepths = numberArray(frame?.executableDepths);

	return symbols.map((symbol, index) => ({
		symbol,
		volume: volumes[index] ?? 0,
		quoteNotional: quoteNotionals[index] ?? 0,
		executableDepth: executableDepths[index] ?? 0,
	}));
};

const median = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const sorted = [...values].sort((left, right) => left - right);
	const middle = Math.floor(sorted.length / 2);

	return sorted.length % 2 === 0
		? (sorted[middle - 1] + sorted[middle]) / 2
		: sorted[middle];
};

export const compactNumber = (value: number): string => {
	const abs = Math.abs(value);

	if (abs >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`;
	if (abs >= 1_000) return `${(value / 1_000).toFixed(1)}K`;

	return abs >= 1 ? value.toFixed(2) : value.toFixed(4);
};

export const crossSectionReadoutFromFrame = (
	frame: Record<string, unknown> | null,
): CrossSectionReadout => {
	const metrics = symbolMetricsFromFrame(frame);

	return {
		leader: typeof frame?.leader === "string" ? frame.leader : "",
		leadershipThresholdPercent: finiteNumber(frame?.leadershipThreshold) * 100,
		breadthPercent: finiteNumber(frame?.breadth) * 100,
		symbolCount: metrics.length,
		medianVolume: median(metrics.map((metric) => metric.volume)),
		medianQuoteNotional: median(metrics.map((metric) => metric.quoteNotional)),
		medianExecutableDepth: median(
			metrics.map((metric) => metric.executableDepth),
		),
	};
};

/*
crossSectionReadoutFromMeasurements derives the panel directly from signal rows
when the backend sends current measurement deltas instead of diagnostics frames.
*/
export const crossSectionReadoutFromMeasurements = (
	measurements: Measurement[],
): CrossSectionReadout => {
	const metrics = symbolMetricsFromMeasurements(measurements);
	const leader = metrics.reduce(
		(best, metric) => (metric.magnitude > best.magnitude ? metric : best),
		{ symbol: "", magnitude: 0 },
	).symbol;

	return {
		leader,
		leadershipThresholdPercent: 0,
		breadthPercent: 0,
		symbolCount: metrics.length,
		medianVolume: median(metrics.map((metric) => metric.volume)),
		medianQuoteNotional: median(metrics.map((metric) => metric.quoteNotional)),
		medianExecutableDepth: median(
			metrics.map((metric) => metric.executableDepth),
		),
	};
};

/*
retainCrossSectionReadout keeps the previous cross-section snapshot when a new
diagnostics frame arrives without peer metrics, so intermittent empty backend
ticks do not flash the panel back to zero.
*/
export const retainCrossSectionReadout = (
	previous: CrossSectionReadout | null,
	incoming: CrossSectionReadout,
): CrossSectionReadout => {
	if (incoming.symbolCount > 0) {
		return incoming;
	}

	if (previous === null || previous.symbolCount === 0) {
		return incoming;
	}

	return previous;
};

const badgeRef = createRef<HTMLSpanElement>();
const subtitleRef = createRef<HTMLDivElement>();
const leaderRef = createRef<HTMLSpanElement>();
const thresholdRef = createRef<HTMLSpanElement>();
const breadthMeterRef = createRef<HTMLDivElement>();
const volumeRef = createRef<HTMLDivElement>();
const notionalRef = createRef<HTMLDivElement>();
const depthRef = createRef<HTMLDivElement>();

let lastReadout: CrossSectionReadout | null = null;

const paintStat = (node: HTMLDivElement | null, value: string) => {
	const valueNode = node?.querySelector<HTMLElement>(
		"[data-stat-value='true']",
	);

	if (valueNode !== null && valueNode !== undefined) {
		valueNode.textContent = value;
	}
};

/*
paintCrossSection paints diagnostics readouts from the current DRAW diagnostics
batch into the CrossSectionPanel shell.
*/
export const paintCrossSection = (value: unknown, _focusSymbol: string) => {
	const rows = Array.isArray(value) ? value : value != null ? [value] : [];
	const frame = rows.at(-1) as DiagnosticsFrame | undefined;
	const measurements = rows.filter(
		(row): row is Measurement =>
			asRecord(row) !== null && typeof asRecord(row)?.symbol === "string",
	);
	const readout = retainCrossSectionReadout(
		lastReadout,
		measurements.length > 0
			? crossSectionReadoutFromMeasurements(measurements)
			: crossSectionReadoutFromFrame(frame ?? null),
	);
	lastReadout = readout;
	const broad = readout.breadthPercent >= 50;

	if (badgeRef.current !== null) {
		badgeRef.current.textContent = broad ? "broad" : "thin";
		badgeRef.current.style.color = broad ? "var(--success)" : "var(--warning)";
		badgeRef.current.style.background = broad
			? "color-mix(in srgb, var(--success) 12%, transparent)"
			: "color-mix(in srgb, var(--warning) 12%, transparent)";
		badgeRef.current.style.borderColor = broad
			? "color-mix(in srgb, var(--success) 38%, transparent)"
			: "color-mix(in srgb, var(--warning) 38%, transparent)";
	}

	if (subtitleRef.current !== null) {
		subtitleRef.current.textContent = `breadth · leadership · liquidity axes · ${readout.symbolCount} symbols`;
	}

	if (leaderRef.current !== null) {
		leaderRef.current.textContent = readout.leader || "—";
	}

	if (thresholdRef.current !== null) {
		thresholdRef.current.textContent = `thr ${readout.leadershipThresholdPercent.toFixed(2)}%`;
	}

	if (breadthMeterRef.current !== null) {
		paintInlineMeter(
			breadthMeterRef.current,
			"Breadth",
			`${Math.round(readout.breadthPercent)}%`,
			readout.breadthPercent,
			broad ? "success" : "warning",
		);
	}

	paintStat(volumeRef.current, compactNumber(readout.medianVolume));
	paintStat(notionalRef.current, compactNumber(readout.medianQuoteNotional));
	paintStat(depthRef.current, compactNumber(readout.medianExecutableDepth));
};

/*
CrossSectionPanel is the static cross-section shell. DRAW paints via
paintCrossSection.
*/
export const CrossSectionPanel = () => (
	<Panel size="lg">
		<div className="flex items-center justify-between">
			<span className="font-semibold text-(--f1) text-xs">Cross-section</span>
			<span
				ref={badgeRef}
				className="rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
			/>
		</div>
		<div
			ref={subtitleRef}
			className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)"
		/>
		<div className="flex items-center justify-between">
			<span className="font-mono text-[11px] text-(--f2)">
				leader <span ref={leaderRef} className="text-(--acc)" />
			</span>
			<span ref={thresholdRef} className="font-mono text-[10px] text-(--f4)" />
		</div>
		<div className="mt-2.5">
			<div
				ref={breadthMeterRef}
				className="flex items-center gap-2"
				role="progressbar"
				aria-valuemin={0}
				aria-valuemax={100}
			>
				<span
					data-inline-label="true"
					className="w-[54px] shrink-0 font-mono text-[9px] text-(--f4)"
				/>
				<div
					data-inline-track="true"
					className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line) [--meter-tone:var(--info)]"
				>
					<div data-inline-fill="true" className="h-full bg-(--meter-tone)" />
				</div>
				<span
					data-inline-value="true"
					className="w-[18px] shrink-0 text-right font-mono text-[9px] text-(--f2)"
				/>
			</div>
		</div>
		<div className="mt-[13px] flex justify-between gap-3">
			<div ref={volumeRef} className="min-w-0 flex-1">
				<div
					data-stat-value="true"
					className="font-mono text-lg text-(--f1) leading-none"
				/>
				<div className="mt-1 font-mono text-[9px] text-(--f4)">med volume</div>
			</div>
			<div ref={notionalRef} className="min-w-0 flex-1 text-center">
				<div
					data-stat-value="true"
					className="font-mono text-lg text-(--f1) leading-none"
				/>
				<div className="mt-1 font-mono text-[9px] text-(--f4)">
					med notional
				</div>
			</div>
			<div ref={depthRef} className="min-w-0 flex-1 text-right">
				<div
					data-stat-value="true"
					className="font-mono text-lg text-(--f1) leading-none"
				/>
				<div className="mt-1 font-mono text-[9px] text-(--f4)">med depth</div>
			</div>
		</div>
	</Panel>
);
