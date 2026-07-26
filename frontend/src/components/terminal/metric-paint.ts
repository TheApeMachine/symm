import type { HeatmapCell } from "#/components/kernel/heatmap";
import { colormapCss, heatmapForeground } from "#/lib/colormap";
import type { Measurement } from "#/collections/types";

const METER_TONE: Record<"info" | "success" | "warning" | "error", string> = {
	info: "var(--info)",
	success: "var(--success)",
	warning: "var(--warning)",
	error: "var(--error)",
};

/*
SOURCE_HEADLINE_METRIC selects the native metric that best summarizes sources
whose primary reading is more specific than the shared strength metric.
*/
const SOURCE_HEADLINE_METRIC: Record<string, string | null> = {
	hawkes: "conditional_intensity",
	liquidity: "scarcity_score",
	toxicity: "touch_quantity",
};

/*
createMetricRow builds one stacked xs meter shell that direct paint can update
without React reconciliation on every websocket tick.
*/
const createMetricRow = (): HTMLDivElement => {
	const row = document.createElement("div");
	row.setAttribute("role", "progressbar");
	row.setAttribute("aria-valuemin", "0");
	row.setAttribute("aria-valuemax", "100");

	const header = document.createElement("div");
	header.className = "mb-1 flex justify-between font-mono text-[9px]";

	const label = document.createElement("span");
	label.className = "text-(--f3)";
	label.dataset.metricLabel = "true";

	const value = document.createElement("span");
	value.className = "text-(--f1)";
	value.dataset.metricValue = "true";

	header.append(label, value);

	const track = document.createElement("div");
	track.className =
		"h-1 overflow-hidden rounded-[2px] bg-(--line) [--meter-tone:var(--info)]";
	track.dataset.metricTrack = "true";

	const fill = document.createElement("div");
	fill.className = "h-full bg-(--meter-tone)";
	fill.dataset.metricFill = "true";

	track.append(fill);
	row.append(header, track);

	return row;
};

/*
paintMetricRow updates one meter shell with the latest measurement readout.
*/
const paintMetricRow = (
	row: HTMLElement,
	measurement: Measurement,
	headline: string | null,
): void => {
	const label = row.querySelector<HTMLElement>("[data-metric-label='true']");
	const value = row.querySelector<HTMLElement>("[data-metric-value='true']");
	const track = row.querySelector<HTMLElement>("[data-metric-track='true']");
	const fill = row.querySelector<HTMLElement>("[data-metric-fill='true']");
	const metric = (measurement.metrics?.[headline ?? ""].normalized || measurement.metrics?.[headline ?? ""].raw) ?? 0;
	const metricLabel = SOURCE_HEADLINE_METRIC[measurement.source] ?? "strength";
	const percent = Math.max(0, Math.min(100, metric * 100));
	const variant = metricLabel === headline ? "warning" : "info";
	const labelText = [
		metricLabel,
		measurement.metrics?.[metricLabel]?.unit !== undefined
			? `(${measurement.metrics?.[metricLabel]?.unit})`
			: null,
	]
		.filter(Boolean)
		.join(" · ");

	if (label !== null) {
		label.textContent = labelText;
	}

	if (value !== null) {
		value.textContent = metric.toFixed(4);
	}

	if (track !== null) {
		track.style.setProperty("--meter-tone", METER_TONE[variant]);
	}

	if (fill !== null) {
		fill.style.width = `${percent}%`;
	}

	row.setAttribute("aria-label", labelText);
	row.setAttribute("aria-valuetext", metric.toFixed(4));
	row.setAttribute("aria-valuenow", String(Math.round(percent)));
};

/*
paintMetricGrid reconciles a metric grid container against the latest epoch so
high-frequency inspector and detail surfaces avoid React child reconciliation.
*/
export const paintMetricGrid = (
	grid: HTMLElement,
	values: Measurement[],
	headline: string | null,
): void => {
	const epoch = values.filter((row) => row.metrics?.[headline ?? ""] !== undefined);
	const existing = new Map<string, HTMLElement>();

	for (const child of grid.children) {
		const key = child.getAttribute("data-metric-key");

		if (key !== null) {
			existing.set(key, child as HTMLElement);
		}
	}

	const orderedRows: HTMLElement[] = [];
	const nextKeys = new Set<string>();

	for (const measurement of epoch) {
		const key = measurement.source;
		nextKeys.add(key);

		let row = existing.get(key);

		if (row === undefined) {
			row = createMetricRow();
			row.setAttribute("data-metric-key", key);
			existing.set(key, row);
		}

		paintMetricRow(row, measurement, headline);
		orderedRows.push(row);
	}

	for (const row of orderedRows) {
		if (row.parentElement !== grid) {
			grid.append(row);
		}
	}

	for (const [key, row] of existing) {
		if (nextKeys.has(key)) {
			continue;
		}

		row.remove();
	}
};

/*
paintHeatmapGrid updates tiles in place. New symbols append; stale symbols
remove. DOM order is never reshuffled on DRAW — batch order must not rebuild
the grid.
*/
export const paintHeatmapGrid = (
	grid: HTMLElement,
	cells: HeatmapCell[],
): void => {
	const existing = new Map<string, HTMLElement>();

	for (const child of grid.children) {
		const symbol = child.getAttribute("data-symbol");

		if (symbol !== null) {
			existing.set(symbol, child as HTMLElement);
		}
	}

	const nextSymbols = new Set<string>();

	for (const cell of cells) {
		nextSymbols.add(cell.symbol);

		let tile = existing.get(cell.symbol);
		const percent = Math.round(cell.value * 100);

		if (tile === undefined) {
			tile = document.createElement("div");
			tile.className =
				"flex aspect-square cursor-pointer items-center justify-center rounded-[2px] font-mono text-[8px]";
			tile.setAttribute("data-symbol", cell.symbol);
			existing.set(cell.symbol, tile);
			grid.append(tile);
		}

		if (tile.textContent !== cell.label) {
			tile.textContent = cell.label;
		}

		tile.title = `${cell.symbol} · ${percent}%`;
		tile.style.background = colormapCss(cell.value);
		tile.style.color = heatmapForeground(cell.value);
	}

	for (const [symbol, tile] of existing) {
		if (nextSymbols.has(symbol)) {
			continue;
		}

		tile.remove();
	}
};

type InlineMeterVariant = keyof typeof METER_TONE;

/*
paintInlineMeter updates one inline meter shell used by health and cross-section.
*/
export const paintInlineMeter = (
	row: HTMLElement | null,
	label: string,
	value: string,
	percent: number,
	variant: InlineMeterVariant,
): void => {
	if (row === null) {
		return;
	}

	const labelNode = row.querySelector<HTMLElement>(
		"[data-inline-label='true']",
	);
	const valueNode = row.querySelector<HTMLElement>(
		"[data-inline-value='true']",
	);
	const track = row.querySelector<HTMLElement>("[data-inline-track='true']");
	const fill = row.querySelector<HTMLElement>("[data-inline-fill='true']");
	const clamped = Math.max(0, Math.min(100, percent));

	if (labelNode !== null) {
		labelNode.textContent = label;
	}

	if (valueNode !== null) {
		valueNode.textContent = value;
	}

	if (track !== null) {
		track.style.setProperty("--meter-tone", METER_TONE[variant]);
	}

	if (fill !== null) {
		fill.style.width = `${clamped}%`;
	}

	row.setAttribute("aria-valuenow", String(Math.round(clamped)));
};
