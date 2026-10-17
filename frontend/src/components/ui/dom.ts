/*
Low-level DOM mutation and query caching utilities for the UI library.

These helpers power zero-React-re-render high-throughput telemetry updates.
*/

/*
meterPercent maps a signed reading onto a 0–100 fill: magnitude × 100,
capped so values above 1 still read as full rather than a sliver.
*/
export const meterPercent = (value: number | null | undefined): number => {
	if (typeof value !== "number" || !Number.isFinite(value)) return 0;
	return Math.min(100, Math.abs(value) * 100);
};

const querySelectorCache = new WeakMap<Element, Map<string, Element | null>>();
const querySelectorAllCache = new WeakMap<Element, Map<string, Element[]>>();
const seenElements = new WeakSet<HTMLElement>();

/*
memoizedQuery caches querySelector results per element and selector.
Use this to eliminate redundant DOM tree walks on repeated queries.
*/
export const memoizedQuery = <T extends Element = HTMLElement>(
	el: Element,
	selector: string,
): T | null => {
	let elementMap = querySelectorCache.get(el);

	if (!elementMap) {
		elementMap = new Map<string, Element | null>();
		querySelectorCache.set(el, elementMap);
	}

	let cached = elementMap.get(selector);

	if (cached === undefined) {
		const found = el.querySelector<T>(selector);
		elementMap.set(selector, found);
		cached = found;
	}

	return (cached as T) ?? null;
};

/*
memoizedQueryAll caches querySelectorAll results per element and selector.
*/
export const memoizedQueryAll = <T extends Element = HTMLElement>(
	el: Element,
	selector: string,
): T[] => {
	let elementMap = querySelectorAllCache.get(el);

	if (!elementMap) {
		elementMap = new Map<string, Element[]>();
		querySelectorAllCache.set(el, elementMap);
	}

	let cached = elementMap.get(selector);

	if (cached === undefined) {
		const found = Array.from(el.querySelectorAll<T>(selector));
		elementMap.set(selector, found);
		cached = found;
	}

	return (cached as T[]) ?? [];
};

/*
setText updates an element's text only when different from current content,
preventing browser layout invalidation and style recalculation.
*/
export const setText = (element: HTMLElement, text: string): void => {
	if (element.textContent !== text) {
		element.textContent = text;
		element.innerText = text;
	}
};

export type ValueFormat =
	| "pct"
	| "bp"
	| "int"
	| "currency"
	| "eur"
	| "pnl"
	| "bits"
	| "nat"
	| "spread"
	| "bool"
	| "time"
	| `fixed:${number}`
	| string;

/*
formatValue formats numerical telemetry values declaratively according
to standard financial and statistical representations.
*/
export const formatValue = (val: number, format?: string | null): string => {
	if (!format) {
		return val.toFixed(4);
	}

	switch (format) {
		case "pct":
			return `${(val * 100).toFixed(1)}%`;
		case "bp":
			return `${(val * 10000).toFixed(1)} bp`;
		case "int":
			return Math.floor(val).toLocaleString();
		case "currency":
		case "eur":
			return `€${val.toFixed(2)}`;
		case "pnl": {
			const sign = val >= 0 ? "+" : "-";
			return `${sign}€${Math.abs(val).toFixed(4)}`;
		}
		case "bits":
			return `${val.toFixed(2)} bits`;
		case "nat":
			return `${val.toFixed(2)} nat`;
		case "spread":
			return `Spread ${val.toFixed(3)}`;
		case "bool":
			return val > 0.5 ? "true" : "false";
		case "time":
			return new Date(val > 1e12 ? val / 1e6 : val * 1e3)
				.toISOString()
				.slice(11, 19);
		default:
			if (format.startsWith("fixed:")) {
				const precision = Number.parseInt(format.slice(6), 10);
				if (!Number.isNaN(precision)) {
					return val.toFixed(precision);
				}
			}
			return val.toFixed(4);
	}
};

import { setBadge } from "./badge";
import { setMeter } from "./meter";

const extractValue = (val: unknown, componentType?: string): unknown => {
	if (val === null || val === undefined) return val;
	if (typeof val === "object") {
		if (componentType === "meter") {
			if ("snm" in val && Number.isFinite((val as { snm: number }).snm)) {
				return (val as { snm: number }).snm;
			}
		}

		if ("raw" in val && typeof (val as { raw: unknown }).raw === "number") {
			return (val as { raw: number }).raw;
		}

		if ("snm" in val && Number.isFinite((val as { snm: number }).snm)) {
			return (val as { snm: number }).snm;
		}

		if ("value" in val && (val as { value: unknown }).value !== undefined) {
			return (val as { value: unknown }).value;
		}
	}

	return val;
};

/*
renderValue writes or paints a value into an element according to its data-component
attribute (defaulting to text).
Supports:
- data-component="text" (default): formats number/string into textContent/innerText
- data-component="meter": sets Meter bar width
- data-component="badge": sets Badge label and status
*/
export const renderValue = (
	element: HTMLElement | null,
	rawValue: unknown,
	format?: string | null,
): void => {
	if (element === null) return;

	const componentType =
		element.dataset.component ??
		(element.hasAttribute("data-metric-meter") ||
		element.dataset.k === "bar" ||
		element.hasAttribute("data-mbar")
			? "meter"
			: element.hasAttribute("data-metric-badge") ||
					element.dataset.k === "badge"
				? "badge"
				: "text");
	const value = extractValue(rawValue, componentType);

	if (componentType === "meter") {
		const num = typeof value === "number" ? value : Number(value);
		if (!Number.isNaN(num) && Number.isFinite(num)) {
			setMeter(element, meterPercent(num), "warning");
		}
		return;
	}

	if (componentType === "badge") {
		setBadge(element, "success", typeof value === "string" ? value : "HEALTHY");
		return;
	}

	if (value === undefined || value === null) {
		if (!seenElements.has(element)) {
			setText(element, element.dataset.fallback ?? "--");
		}
		return;
	}

	let out: string;

	if (typeof value === "number") {
		if (Number.isNaN(value) || !Number.isFinite(value)) {
			return;
		}
		const fmt = format ?? element.dataset.format ?? null;
		out = formatValue(value, fmt);
	} else if (typeof value === "string") {
		const fmt = format ?? element.dataset.format ?? null;
		const num = Number(value);
		if (fmt && !Number.isNaN(num) && Number.isFinite(num) && value.trim() !== "") {
			out = formatValue(num, fmt);
		} else {
			out = value;
		}
	} else if (typeof value === "boolean") {
		out = value ? "TRUE" : "FALSE";
	} else if (typeof value === "bigint") {
		const fmt = format ?? element.dataset.format ?? null;
		if (fmt === "time" || (fmt === null && element.dataset.metric === "at")) {
			out = new Date(Number(value / 1000000n)).toISOString().slice(11, 19);
		} else {
			out = Number(value).toLocaleString();
		}
	} else {
		out = (value as { toString?: () => string }).toString?.() ?? "";
	}

	seenElements.add(element);
	setText(element, out);
};

export const resetValue = (element: HTMLElement, fallback = "--"): void => {
	const componentType =
		element.dataset.component ??
		(element.hasAttribute("data-metric-meter") ||
		element.dataset.k === "bar" ||
		element.hasAttribute("data-mbar")
			? "meter"
			: element.hasAttribute("data-metric-badge") ||
					element.dataset.k === "badge"
				? "badge"
				: "text");

	if (componentType === "meter") {
		setMeter(element, 0, "disabled");
	} else if (componentType === "badge") {
		setBadge(element, "disabled", "STANDBY");
	} else {
		setText(element, element.dataset.fallback ?? fallback);
	}
};
