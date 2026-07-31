import { appStore, DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import type { Measurement } from "#/collections/types";
import {
	type MeterParts,
	mergeInspectorMetrics,
	paintInspectorMeters,
} from "#/components/terminal/inspector-meters";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Component } from "../ui/component";

type SparkView = {
	line: SVGPolylineElement | null;
	area: SVGPolylineElement | null;
};

type KernelRowModel = {
	source: string;
	status: string;
	statusDot: string;
	readout: string;
	age: string;
	barWidth: string;
};

type KernelPaintModel = {
	rows: KernelRowModel[];
};

type RowParts = {
	root: HTMLButtonElement;
	compact: boolean;
	samples: number[];
	sparks: SparkView[];
	pressed: string;
};

type InspectorParts = {
	root: HTMLElement;
	source: string | null;
	count: number;
	statusText: string;
	title: HTMLElement | null;
	status: HTMLElement | null;
	series: HTMLElement | null;
	sparkLine: SVGPolylineElement | null;
	metrics: HTMLElement | null;
	waiting: HTMLElement | null;
	symbol: HTMLElement | null;
	observed: HTMLElement | null;
	meters: Map<string, MeterParts>;
};

const STATUS_HEALTHY_CLASSES = [
	"border-[color-mix(in_srgb,var(--up)_38%,transparent)]",
	"bg-[color-mix(in_srgb,var(--up)_12%,transparent)]",
	"text-(--up)",
];
const STATUS_INVALID_CLASSES = [
	"border-[color-mix(in_srgb,var(--down)_38%,transparent)]",
	"bg-[color-mix(in_srgb,var(--down)_12%,transparent)]",
	"text-(--down)",
];

const HEADLINE_METRIC: Record<string, string> = {
	hawkes: "conditional_intensity",
	liquidity: "scarcity_score",
	toxicity: "touch_quantity",
};

const STATUS_DOT: Record<string, string> = {
	HEALTHY: "var(--up)",
	INVALID: "var(--down)",
	STANDBY: "var(--f3)",
};

/*
kernelListReadout selects one native headline metric from a flat measurement row
so the list can paint directly without depending on the removed measurement view.
*/
const kernelListReadout = (row: Measurement) => {
	const metric = HEADLINE_METRIC[row.source] ?? "strength";
	const reading =
		Object.entries(row.metrics ?? {})
			.filter(([key]) => key === metric || key.startsWith(`${metric}:`))
			.map(([, value]) => value)
			.sort(
				(left, right) =>
					(right.normalized ?? right.raw) - (left.normalized ?? left.raw),
			)
			.at(0) ?? row.metrics?.strength;
	const raw = reading?.raw ?? 0;
	const sample = Math.max(0, Math.min(1, reading?.normalized ?? raw));

	return { metric, raw, sample };
};

const measurementRows = (value: unknown) =>
	(Array.isArray(value) ? value : value != null ? [value] : []) as Measurement[];

const ageLabel = (at?: string) => {
	if (!at) {
		return "";
	}

	const elapsed = Date.now() - Date.parse(at);

	if (!Number.isFinite(elapsed)) {
		return "";
	}

	if (elapsed < 60_000) {
		return `${Math.max(0, Math.floor(elapsed / 1000))}s`;
	}

	return `${Math.floor(elapsed / 60_000)}m`;
};

const kernelStatus = (row?: Measurement) => {
	if (row === undefined) {
		return "STANDBY";
	}

	return row.validity?.state !== "invalid" ? "HEALTHY" : "INVALID";
};

const kernelModel = (
	value: unknown,
	focusSymbol: string,
	sources: string[],
): KernelPaintModel => {
	const latest = new Map<string, Measurement>();
	const observed = new Set<string>();

	for (const row of measurementRows(value)) {
		if (!row || typeof row.source !== "string") {
			continue;
		}

		observed.add(row.source);

		if (focusSymbol !== "" && row.symbol !== focusSymbol) {
			continue;
		}

		const current = latest.get(row.source);

		if (current === undefined || Date.parse(current.at) <= Date.parse(row.at)) {
			latest.set(row.source, row);
		}
	}

	appStore.actions.observeSources(observed);

	return {
		rows: sources.map((source) => {
			const row = latest.get(source);
			const status = kernelStatus(row);

			if (row === undefined) {
				return {
					source,
					status,
					statusDot: STATUS_DOT[status],
					readout: "waiting",
					age: "",
					barWidth: "0%",
				};
			}

			const { metric, raw, sample } = kernelListReadout(row);

			return {
				source,
				status,
				statusDot: STATUS_DOT[status],
				readout: `${metric} ${raw.toPrecision(4)}`,
				age: ageLabel(row.at),
				barWidth: `${Math.max(0, Math.min(100, sample * 100))}%`,
			};
		}),
	};
};

/*
syncWaiting toggles the metrics spinner while DRAW has not yet painted meters.
*/
const syncWaiting = () => {
	if (inspector === null || inspector.waiting === null) {
		return;
	}

	inspector.waiting.hidden = inspector.meters.size > 0;
};

export const kernelRows = new Map<string, RowParts>();
let inspector: InspectorParts | null = null;

/*
sparkPoints builds the SVG points string from the in-memory y-sample series.
*/
const sparkPoints = (samples: number[]): string =>
	samples
		.map((sampleY, index) => {
			const x = samples.length === 1 ? 0 : (index / (samples.length - 1)) * 150;

			return `${x.toFixed(1)},${sampleY.toFixed(1)}`;
		})
		.join(" ");

/*
writeSpark paints polylines from an existing sample series without appending.
*/
const writeSpark = (
	line: SVGPolylineElement | null,
	area: SVGPolylineElement | null,
	samples: number[],
) => {
	if (samples.length === 0) {
		return;
	}

	const spark = sparkPoints(samples);
	const y = samples.at(-1)?.toFixed(1) ?? "0.0";

	if (line !== null) {
		line.setAttribute("points", spark);
	}

	if (area !== null) {
		area.setAttribute(
			"points",
			samples.length === 1
				? `${spark} 150,${y} 150,30 0,30`
				: `${spark} 150,30 0,30`,
		);
	}
};

/*
appendSpark pushes one unit sample into the series and caps length at 50.
*/
const appendSpark = (samples: number[], unit: number) => {
	samples.push(29 - unit * 26);

	while (samples.length > 50) {
		samples.shift();
	}
};

const syncPressedRows = () => {
	const { inspectorSource, selectedSource } = terminalStore.state;

	for (const [source, parts] of kernelRows) {
		const pressed =
			inspectorSource === source || selectedSource === source ? "true" : "false";

		if (parts.pressed === pressed) {
			continue;
		}

		parts.pressed = pressed;
		parts.root.classList.toggle("border-l-(--acc)", pressed === "true");
		parts.root.classList.toggle("bg-(--raised)", pressed === "true");
		parts.root.classList.toggle("border-l-transparent", pressed !== "true");
		parts.root.classList.toggle("bg-transparent", pressed !== "true");
		parts.root.setAttribute("aria-pressed", pressed);
	}
};

const detachInspectorSpark = () => {
	if (
		inspector === null ||
		inspector.source === null ||
		inspector.sparkLine === null
	) {
		return;
	}

	const row = kernelRows.get(inspector.source);

	if (row === undefined) {
		return;
	}

	row.sparks = row.sparks.filter(
		(spark) => spark.line !== inspector?.sparkLine,
	);
};

const attachInspectorSpark = (source: string) => {
	if (inspector === null || inspector.sparkLine === null) {
		return;
	}

	const row = kernelRows.get(source);

	if (row === undefined) {
		return;
	}

	if (!row.sparks.some((spark) => spark.line === inspector?.sparkLine)) {
		row.sparks.push({ line: inspector.sparkLine, area: null });
	}

	writeSpark(inspector.sparkLine, null, row.samples);
};

/*
syncInspector shows or hides the single inspector shell and rebinds its spark
view when the inspected source changes.
*/
export const syncInspector = (source: string | null) => {
	if (inspector === null) {
		return;
	}

	if (source === null) {
		detachInspectorSpark();
		inspector.source = null;
		inspector.root.hidden = true;
		return;
	}

	if (inspector.source !== source) {
		detachInspectorSpark();

		for (const meter of inspector.meters.values()) {
			meter.cell.remove();
		}

		inspector.meters.clear();
		inspector.statusText = "";
		inspector.count = kernelRows.get(source)?.samples.length ?? 0;
		inspector.source = source;
		syncWaiting();

		if (inspector.title !== null) {
			inspector.title.textContent = source;
		}

		attachInspectorSpark(source);
	}

	inspector.root.hidden = false;
};

/*
openInspectorShell selects a source and paints the inspector immediately.
*/
export const openInspectorShell = (source: string) => {
	terminalStore.actions.inspectSource(source);
	syncInspector(source);
	syncPressedRows();
};

/*
closeInspectorShell clears the inspected source and hides the shell.
*/
export const closeInspectorShell = () => {
	terminalStore.actions.closeInspect();
	syncInspector(null);
	syncPressedRows();
};

/*
bindInspector binds the persistent inspector shell once. Open/close is painted
from click handlers and DRAW via syncInspector — no store subscription.
*/
export const bindInspector = (root: HTMLElement | null) => {
	if (root === null) {
		detachInspectorSpark();
		inspector = null;
		return;
	}

	if (inspector?.root === root) {
		return;
	}

	inspector = {
		root,
		source: null,
		count: 0,
		statusText: "",
		title: root.querySelector('[data-role="title"]'),
		status: root.querySelector('[data-role="status"]'),
		series: root.querySelector('[data-role="series"]'),
		sparkLine: root.querySelector('[data-role="spark-line"]'),
		metrics: root.querySelector('[data-role="metrics"]'),
		waiting: root.querySelector('[data-role="metrics-waiting"]'),
		symbol: root.querySelector('[data-role="symbol"]'),
		observed: root.querySelector('[data-role="observed"]'),
		meters: new Map(),
	};

	root.hidden = true;
	syncInspector(terminalStore.state.inspectorSource);
};

/*
paintKernelList keeps the sparkline and inspector incremental while Component
handles the granular row text and style updates.
*/
export const paintKernelList = (value: unknown, focusSymbol: string) => {
	if (kernelRows.size === 0 && inspector?.source == null) {
		return;
	}

	syncInspector(terminalStore.state.inspectorSource);
	syncPressedRows();

	const rows = measurementRows(value);
	const observed = new Set<string>();
	const now = Date.now();

	for (const row of rows) {
		if (typeof row.source === "string") {
			observed.add(row.source);
		}

		if (focusSymbol !== "" && row.symbol !== focusSymbol) {
			continue;
		}

		const { metric, raw, sample } = kernelListReadout(row);
		const valid = row.validity?.state !== "invalid";
		const statusText = valid ? "Healthy" : "Invalid";
		const elapsed = now - Date.parse(row.at);
		const age = !Number.isFinite(elapsed)
			? "—"
			: elapsed < 60_000
				? `${Math.max(0, Math.floor(elapsed / 1000))}s`
				: `${Math.floor(elapsed / 60_000)}m`;

		const parts = kernelRows.get(row.source);

		if (parts !== undefined) {
			appendSpark(parts.samples, sample);

			for (const spark of parts.sparks) {
				writeSpark(spark.line, spark.area, parts.samples);
			}
		}

		if (inspector === null || inspector.source !== row.source) {
			continue;
		}

		inspector.count += 1;

		if (inspector.status !== null && inspector.statusText !== statusText) {
			inspector.statusText = statusText;
			inspector.status.textContent = statusText;
			inspector.status.classList.remove(...STATUS_HEALTHY_CLASSES);
			inspector.status.classList.remove(...STATUS_INVALID_CLASSES);
			inspector.status.classList.add(
				...(valid ? STATUS_HEALTHY_CLASSES : STATUS_INVALID_CLASSES),
			);
		}

		if (inspector.series !== null) {
			inspector.series.textContent = metric;
		}

		if (inspector.symbol !== null) {
			inspector.symbol.textContent = `active ${row.symbol}`;
		}

		if (inspector.observed !== null) {
			inspector.observed.textContent = `observed ${new Date(row.at).toLocaleTimeString("en-US", { hour12: false })} · ${inspector.count} samples`;
		}

		void raw;
		void age;
	}

	appStore.actions.observeSources(observed);

	if (
		inspector !== null &&
		inspector.source !== null &&
		inspector.metrics !== null
	) {
		const merged = mergeInspectorMetrics(rows, inspector.source, focusSymbol);

		if (merged.size > 0) {
			paintInspectorMeters(inspector.metrics, inspector.meters, merged);
			syncWaiting();
		}
	}
};

/*
KernelList mounts one stable row per source and lets Component paint direct
value updates in place while paintKernelList keeps the sparkline incremental.
*/
export const KernelList = ({
	compact = false,
	sources = DEFAULT_KERNELS,
}: {
	compact?: boolean;
	sources?: string[];
}) => (
	<Component
		register={(paint) =>
			registerPainter("measurements", (updates) => {
				paint(kernelModel(updates, appStore.state.focusSymbol, sources));
			})
		}
		select="rows"
	>
		{({ ref, className }) => (
			<div ref={ref} className={cn("min-h-0 overflow-auto", className)}>
				{sources.map((source, index) => (
					<button
						key={source}
						type="button"
						data-index={index}
						onClick={() => {
							if (compact) {
								terminalStore.actions.selectSource(source);
								syncPressedRows();
								return;
							}

							openInspectorShell(source);
						}}
						ref={(element) => {
							if (element === null) {
								kernelRows.delete(source);
								return;
							}

							const current = kernelRows.get(source);

							kernelRows.set(source, {
								root: element,
								compact,
								samples: current?.samples ?? [],
								sparks: [
									{
										line: element.querySelector('[data-role="spark-line"]'),
										area: element.querySelector('[data-role="spark-area"]'),
									},
								],
								pressed: current?.pressed ?? "",
							});

							if (inspector?.source === source) {
								attachInspectorSpark(source);
							}

							syncPressedRows();
						}}
						className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
					>
						<div className="flex items-center justify-between gap-2">
							<span
								data-paint="source"
								className={cn("truncate font-semibold text-(--f1)", {
									"text-xs": compact,
									"text-[12.5px]": !compact,
								})}
							>
								{source}
							</span>

							{compact ? (
								<span
									data-set="statusDot"
									data-target="style.backgroundColor"
									className="size-1.75 shrink-0 rounded-full bg-(--f3)"
								/>
							) : (
								<span
									data-paint="status"
									data-paint-class="HEALTHY:border-[color-mix(in_srgb,var(--up)_38%,transparent)],bg-[color-mix(in_srgb,var(--up)_12%,transparent)],text-(--up) INVALID:border-[color-mix(in_srgb,var(--down)_38%,transparent)],bg-[color-mix(in_srgb,var(--down)_12%,transparent)],text-(--down) STANDBY:border-(--line2),bg-(--line),text-(--f3)"
									className="shrink-0 rounded-xs border border-(--line2) bg-(--line) px-1.25 py-0.5 font-mono text-[9px] uppercase tracking-[0.07em] text-(--f3)"
								>
									STANDBY
								</span>
							)}
						</div>

						{compact ? null : (
							<>
								<svg
									viewBox="0 0 150 30"
									preserveAspectRatio="none"
									className="mt-1.5 block h-6.5 w-full"
								>
									<title>Signal sparkline</title>
									<polyline
										data-role="spark-area"
										className="fill-[color-mix(in_srgb,var(--acc)_16%,transparent)]"
										stroke="none"
									/>
									<polyline
										data-role="spark-line"
										className="stroke-(--acc)"
										fill="none"
										strokeWidth="1.4"
										vectorEffect="non-scaling-stroke"
									/>
								</svg>

								<div className="mt-1.5 flex items-center gap-2">
									<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line)">
										<div
											data-set="barWidth"
											data-target="style.width"
											className="h-full w-0 bg-(--warning) transition-[width] duration-500 ease-out"
										/>
									</div>

									<span
										data-paint="readout"
										className="flex-1 truncate text-right font-mono text-[10px] text-(--f2)"
									>
										waiting
									</span>

									<span
										data-paint="age"
										className="w-11.5 shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
									/>
								</div>
							</>
						)}

						{compact ? (
							<div
								data-paint="readout"
								className="mt-1 truncate font-mono text-[9px] text-(--f4)"
							>
								waiting
							</div>
						) : null}
					</button>
				))}
			</div>
		)}
	</Component>
);
