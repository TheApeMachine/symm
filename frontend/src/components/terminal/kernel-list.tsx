import { DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { repaintSignalDetail } from "#/components/kernel/detail";
import {
	type MeterParts,
	mergeInspectorMetrics,
	paintInspectorMeters,
} from "#/components/terminal/inspector-meters";
import { KernelListRow } from "#/components/terminal/kernel-list-row";
import type { Measurement } from "#/collections/types";

type SparkView = {
	line: SVGPolylineElement | null;
	area: SVGPolylineElement | null;
};

type RowParts = {
	root: HTMLButtonElement;
	compact: boolean;
	samples: number[];
	sparks: SparkView[];
	pressed: string;
	statusText: string;
	status: HTMLElement | null;
	bar: HTMLElement | null;
	readout: HTMLElement | null;
	age: HTMLElement | null;
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

/*
kernelListReadout selects one native headline metric from a flat measurement row
so the list can paint directly without depending on the removed measurement view.
*/
const kernelListReadout = (row: Measurement) => {
	const metric = HEADLINE_METRIC[row.source] ?? "strength";
	const reading = Object.entries(row.metrics ?? {})
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
			const x =
				samples.length === 1
					? 0
					: (index / (samples.length - 1)) * 150;

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

const detachInspectorSpark = () => {
	if (inspector === null || inspector.source === null || inspector.sparkLine === null) {
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
};

/*
closeInspectorShell clears the inspected source and hides the shell.
*/
export const closeInspectorShell = () => {
	terminalStore.actions.closeInspect();
	syncInspector(null);
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
paintKernelList paints each DRAW measurement onto cached row/inspector nodes.
*/
export const paintKernelList = (value: unknown, focusSymbol: string) => {
	const { inspectorSource, selectedSource } = terminalStore.state;
	syncInspector(inspectorSource);
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Measurement[];
	const now = Date.now();

	for (const row of rows) {
		if (focusSymbol !== "" && row.symbol !== focusSymbol) {
			continue;
		}

		const { metric, raw, sample } = kernelListReadout(row);
		const valid = row.validity?.state !== "invalid";
		const statusText = valid ? "Healthy" : "Invalid";
		const percent = Math.max(0, Math.min(100, sample * 100));
		const elapsed = now - Date.parse(row.at);
		const age = !Number.isFinite(elapsed)
			? "—"
			: elapsed < 60_000
				? `${Math.max(0, Math.floor(elapsed / 1000))}s`
				: `${Math.floor(elapsed / 60_000)}m`;

		const parts = kernelRows.get(row.source);

		if (parts !== undefined) {
			const pressed =
				inspectorSource === row.source || selectedSource === row.source
					? "true"
					: "false";

			if (parts.pressed !== pressed) {
				parts.pressed = pressed;
				parts.root.classList.toggle("border-l-(--acc)", pressed === "true");
				parts.root.classList.toggle("bg-(--raised)", pressed === "true");
				parts.root.classList.toggle("border-l-transparent", pressed !== "true");
				parts.root.classList.toggle("bg-transparent", pressed !== "true");
				parts.root.setAttribute("aria-pressed", pressed);
			}

			if (parts.status !== null && parts.statusText !== statusText) {
				parts.statusText = statusText;

				if (parts.compact) {
					parts.status.classList.toggle("bg-(--up)", valid);
					parts.status.classList.toggle("bg-(--down)", !valid);
				} else {
					parts.status.textContent = statusText;
					parts.status.classList.remove(...STATUS_HEALTHY_CLASSES);
					parts.status.classList.remove(...STATUS_INVALID_CLASSES);
					parts.status.classList.add(
						...(valid ? STATUS_HEALTHY_CLASSES : STATUS_INVALID_CLASSES),
					);
				}
			}

			appendSpark(parts.samples, sample);

			for (const spark of parts.sparks) {
				writeSpark(spark.line, spark.area, parts.samples);
			}

			if (parts.bar !== null) {
				parts.bar.style.width = `${percent}%`;
			}

			if (parts.readout !== null) {
				parts.readout.textContent = `${metric} ${raw.toPrecision(4)}`;
			}

			if (parts.age !== null) {
				parts.age.textContent = age;
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
	}

	if (
		inspector !== null &&
		inspector.source !== null &&
		inspector.metrics !== null
	) {
		const merged = mergeInspectorMetrics(
			rows,
			inspector.source,
			focusSymbol,
		);

		if (merged.size > 0) {
			paintInspectorMeters(inspector.metrics, inspector.meters, merged);
			syncWaiting();
		}
	}
};

/*
KernelList mounts one row per kernel and caches its paint targets once.
*/
export const KernelList = ({
	compact = false,
	sources = DEFAULT_KERNELS,
}: {
	compact?: boolean;
	sources?: string[];
}) => (
	<div className="min-h-0 overflow-auto">
		{sources.map((source) => (
			<KernelListRow
				key={source}
				source={source}
				compact={compact}
				onActivate={(next) => {
					if (!compact) {
						openInspectorShell(next);
						return;
					}

					terminalStore.actions.selectSource(next);
					repaintSignalDetail();
				}}
				rowRef={(element) => {
					if (element === null) {
						kernelRows.delete(source);
						return;
					}

					kernelRows.set(source, {
						root: element,
						compact,
						samples: [],
						sparks: [
							{
								line: element.querySelector(
									'[data-role="spark-line"]',
								),
								area: element.querySelector(
									'[data-role="spark-area"]',
								),
							},
						],
						pressed: "",
						statusText: "",
						status: element.querySelector('[data-role="status"]'),
						bar: element.querySelector('[data-role="bar"]'),
						readout: element.querySelector('[data-role="readout"]'),
						age: element.querySelector('[data-role="age"]'),
					});

					if (inspector?.source === source) {
						attachInspectorSpark(source);
					}
				}}
			/>
		))}
	</div>
);
