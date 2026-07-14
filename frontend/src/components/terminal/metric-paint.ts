import type { HeatmapCell } from "#/components/kernel/heatmap";
import { colormapCss, heatmapForeground } from "#/lib/colormap";
import type { Measurement } from "#/types/measurement";
import {
	formatRaw,
	measurementIdentity,
	metricLabel,
	orderedEpoch,
	percentOf,
	sideLabel,
} from "./measurement-view";

const METER_TONE: Record<"info" | "success" | "warning" | "error", string> = {
	info: "var(--info)",
	success: "var(--success)",
	warning: "var(--warning)",
	error: "var(--error)",
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
	const percent = percentOf(measurement);
	const variant = measurement.metric === headline ? "warning" : "info";
	const labelText = [
		metricLabel(measurement.metric),
		sideLabel(measurement.side),
	]
		.filter(Boolean)
		.join(" · ");
	const valueText = formatRaw(measurement);

	if (label !== null) {
		label.textContent = labelText;
	}

	if (value !== null) {
		value.textContent = valueText;
	}

	if (track !== null) {
		track.style.setProperty("--meter-tone", METER_TONE[variant]);
	}

	if (fill !== null) {
		fill.style.width = `${percent}%`;
	}

	row.setAttribute("aria-label", labelText);
	row.setAttribute("aria-valuetext", valueText);
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
	const epoch = orderedEpoch(values, headline);
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
		const key = measurementIdentity(measurement);
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

	for (const [key, row] of existing) {
		if (!nextKeys.has(key)) {
			row.remove();
		}
	}

	const orderMatches =
		orderedRows.length === grid.children.length &&
		orderedRows.every((row, index) => grid.children[index] === row);

	if (!orderMatches) {
		grid.replaceChildren(...orderedRows);
	}
};

/*
paintHeatmapGrid reconciles cross-section heatmap cells without React children.
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
	const orderedTiles: HTMLElement[] = [];

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
		}

		tile.textContent = cell.label;
		tile.title = `${cell.symbol} · ${percent}%`;
		tile.style.background = colormapCss(cell.value);
		tile.style.color = heatmapForeground(cell.value);
		orderedTiles.push(tile);
	}

	for (const [symbol, tile] of existing) {
		if (!nextSymbols.has(symbol)) {
			tile.remove();
		}
	}

	for (const tile of orderedTiles) {
		grid.appendChild(tile);
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
