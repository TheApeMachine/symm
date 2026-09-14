import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export const cn = (...inputs: ClassValue[]): string => {
	return twMerge(clsx(inputs));
};

export const formatPnl = (value: number): string => {
	const sign = value >= 0 ? "+" : "";
	return `${sign}€${value.toFixed(4)}`;
};

export const formatEur = (value: number): string => {
	return `€${value.toFixed(2)}`;
};

export const pnlTone = (value: number | undefined): string => {
	if (value === undefined) return "";
	if (value > 0) return "text-(--dash-up)";
	if (value < 0) return "text-(--dash-down)";
	return "text-(--dash-muted)";
};

export const isValid = (input: number | undefined | null) => {
	if (typeof input !== "number") return false;
	if (isNaN(input)) return false;
	if (input === null) return false;
	return input !== undefined;
};

const seen: WeakSet<HTMLElement> = new WeakSet();

export const renderValue = (
	element: HTMLElement,
	value: string | number | null | undefined,
) => {
	let out: string;

	if (!seen.has(element) && value === undefined) {
		element.innerText = "--";
		return;
	}

	switch (typeof value) {
		case "number":
			if (!isValid(value as number)) {
				return
			}

			out = (value as number).toFixed(4);
			break
		case "string":
			out = value as string
			break
		default:
			if (seen.has(element)) {
				return;
			}

			out = "--";
			break
	}

	if (!seen.has(element)) {
		seen.add(element);
	}

	element.innerText = out;
};

const querySelectorCache = new WeakMap<HTMLElement, Map<string, Element>>();

/*
elementCache takes a selector as a key and an element as the value.
Use this to prevent repeat querySelector calls to the DOM.
*/
export const memoizedQuery = (el: HTMLElement, selector: string) => {
	let elementMap = querySelectorCache.get(el);

	if (!elementMap) {
		elementMap = new Map<string, Element>();
		querySelectorCache.set(el, elementMap);
	}

	let cached = elementMap.get(selector);

	if (!cached) {
		const found = el.querySelector(selector);
		if (found) {
			elementMap.set(selector, found);
			cached = found;
		}
	}

	return cached ?? null;
};
