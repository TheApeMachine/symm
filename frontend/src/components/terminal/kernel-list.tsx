import { DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { repaintSignalDetail } from "#/components/kernel/detail";
import { KernelListRow } from "#/components/terminal/kernel-list-row";
import type { Measurement } from "#/types/measurement";

type MeterParts = {
	cell: HTMLElement;
	value: HTMLElement;
	fill: HTMLElement;
};

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
		line.setAttribute("stroke", "var(--acc)");
	}

	if (area !== null) {
		area.setAttribute(
			"points",
			samples.length === 1
				? `${spark} 150,${y} 150,30 0,30`
				: `${spark} 150,30 0,30`,
		);
		area.setAttribute(
			"fill",
			"color-mix(in srgb, var(--acc) 16%, transparent)",
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

		const rawCandidate =
			typeof row.raw === "number" && Number.isFinite(row.raw)
				? row.raw
				: Object.values(row.metrics ?? {}).find((entry) =>
						Number.isFinite(entry),
					);
		const raw = Number.isFinite(rawCandidate) ? Number(rawCandidate) : 0;
		const sample = Math.max(
			0,
			Math.min(
				1,
				typeof row.normalized === "number" &&
					Number.isFinite(row.normalized)
					? row.normalized
					: Math.abs(raw),
			),
		);
		const valid = row.validity?.state !== "invalid";
		const statusText = valid ? "Healthy" : "Invalid";
		const tone = valid ? "var(--up)" : "var(--down)";
		const metric = row.metric ?? Object.keys(row.metrics ?? {})[0] ?? "raw";
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
				parts.root.style.borderLeftColor =
					pressed === "true" ? "var(--acc)" : "transparent";
				parts.root.style.background =
					pressed === "true" ? "var(--raised)" : "transparent";
				parts.root.setAttribute("aria-pressed", pressed);
			}

			if (parts.status !== null && parts.statusText !== statusText) {
				parts.statusText = statusText;

				if (parts.compact) {
					parts.status.style.backgroundColor = tone;
				} else {
					parts.status.textContent = statusText;
					parts.status.style.color = tone;
					parts.status.style.borderColor =
						`color-mix(in srgb, ${tone} 38%, transparent)`;
					parts.status.style.background =
						`color-mix(in srgb, ${tone} 12%, transparent)`;
				}
			}

			appendSpark(parts.samples, sample);

			for (const spark of parts.sparks) {
				writeSpark(spark.line, spark.area, parts.samples);
			}

			if (parts.bar !== null) {
				parts.bar.style.width = `${percent}%`;
				parts.bar.style.background = "var(--warning)";
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
			inspector.status.style.color = tone;
			inspector.status.style.borderColor =
				`color-mix(in srgb, ${tone} 38%, transparent)`;
			inspector.status.style.background =
				`color-mix(in srgb, ${tone} 12%, transparent)`;
		}

		if (inspector.series !== null) {
			inspector.series.textContent = metric;
		}

		if (inspector.metrics !== null) {
			const entries =
				row.metrics !== undefined
					? Object.entries(row.metrics)
					: ([[metric, raw]] as Array<[string, number]>);
			const currentKeys = new Set<string>();

			for (const [key, entry] of entries) {
				currentKeys.add(key);

				let meter = inspector.meters.get(key);

				if (meter === undefined) {
					const cell = document.createElement("div");
					cell.dataset.metric = key;
					cell.setAttribute("role", "progressbar");
					cell.setAttribute("aria-valuemin", "0");
					cell.setAttribute("aria-valuemax", "100");

					const header = document.createElement("div");
					header.className =
						"mb-1 flex justify-between font-mono text-[9px]";
					const label = document.createElement("span");
					const valueEl = document.createElement("span");
					label.className = "text-(--f3)";
					valueEl.className = "text-(--f1)";
					label.textContent = key;
					header.append(label, valueEl);

					const track = document.createElement("div");
					track.className =
						"h-1 overflow-hidden rounded-[2px] bg-(--line) [--meter-tone:var(--info)]";
					const fill = document.createElement("div");
					fill.className = "h-full bg-(--meter-tone)";
					track.append(fill);
					cell.append(header, track);
					inspector.metrics.append(cell);

					meter = { cell, value: valueEl, fill };
					inspector.meters.set(key, meter);
				}

				const numeric = Number(entry);
				const safe = Number.isFinite(numeric) ? numeric : 0;
				const fillPercent = Math.max(0, Math.min(100, safe * 100));

				meter.value.textContent = safe.toPrecision(4);
				meter.fill.style.width = `${fillPercent}%`;
				meter.cell.setAttribute(
					"aria-valuenow",
					String(Math.round(fillPercent)),
				);
			}

			for (const [key, meter] of inspector.meters) {
				if (currentKeys.has(key)) {
					continue;
				}

				meter.cell.remove();
				inspector.meters.delete(key);
			}

			syncWaiting();
		}

		if (inspector.symbol !== null) {
			inspector.symbol.textContent = `active ${row.symbol}`;
		}

		if (inspector.observed !== null) {
			inspector.observed.textContent = `observed ${new Date(row.at).toLocaleTimeString("en-US", { hour12: false })} · ${inspector.count} samples`;
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
