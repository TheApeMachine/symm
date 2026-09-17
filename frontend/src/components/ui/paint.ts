import { type DependencyList, type RefObject, useEffect, useRef } from "react";
import { type BadgeSize, type BadgeVariant, setBadge } from "./badge";
import { type MeterVariant, setMeter } from "./meter";

export type JSONPrimitive = string | number | boolean | null;

export type JSONSerializable =
	| JSONPrimitive
	| JSONSerializable[]
	| { [key: string]: JSONSerializable | undefined };

export type Paint = (updates: JSONSerializable) => void;

/**
 * Universal interface for stores that can be subscribed to (TanStack Store, Atoms, etc.).
 */
export type SubscribableStore<TState> = {
	state?: TState;
	get?: () => TState;
	subscribe: (
		listener: (state: TState) => void,
	) => { unsubscribe: () => void } | (() => void);
};

export type PaintFieldRecord = Record<string, unknown>;

export type PaintMeterValue =
	| number
	| { percent: number; variant?: MeterVariant };

export type PaintMeterRecord = Record<string, PaintMeterValue>;

export type PaintBadgeValue = {
	label?: string;
	variant?: BadgeVariant;
	size?: BadgeSize;
};

export type PaintBadgeRecord = Record<string, PaintBadgeValue>;

export type PaintVarRecord = Record<string, string | number>;

export type PaintMap = {
	/** Updates textContent of elements matching [data-f="key"] or [data-paint="key"] */
	fields?: PaintFieldRecord;
	/** Updates progress/meter width and aria attributes on elements matching [data-meter="key"] or [data-${key}] */
	meters?: PaintMeterRecord;
	/** Updates badge variant and label on elements matching [data-badge="key"] */
	badges?: PaintBadgeRecord;
	/** Updates CSS custom properties on matching elements [data-axis="key"], [data-var="key"], or root */
	vars?: PaintVarRecord;
	/** Custom direct-DOM mutator for canvas or specialized elements */
	custom?: (root: HTMLElement) => void;
};

/**
 * Applies a PaintMap directly onto a DOM subtree without triggering React re-renders.
 */
export const applyPaintMap = (
	root: HTMLElement,
	map: PaintMap,
	cache?: Map<string, Element | null>,
): void => {
	const query = (selector: string): Element | null => {
		if (cache) {
			const cached = cache.get(selector);
			if (cached !== undefined) return cached;
			const el = root.querySelector(selector);
			cache.set(selector, el);
			return el;
		}
		return root.querySelector(selector);
	};

	if (map.fields) {
		for (const [key, rawValue] of Object.entries(map.fields)) {
			const text =
				rawValue === undefined || rawValue === null ? "—" : String(rawValue);
			const selector = `[data-f="${key}"], [data-paint="${key}"]`;
			const el = query(selector);
			if (el instanceof HTMLElement) {
				if (el.textContent !== text) {
					el.textContent = text;
				}
			}
		}
	}

	if (map.meters) {
		for (const [key, value] of Object.entries(map.meters)) {
			const percent = typeof value === "number" ? value : value.percent;
			const variant = typeof value === "object" ? value.variant : undefined;
			const selector = `[data-meter="${key}"], [data-${key}]`;
			const el = query(selector);
			if (el instanceof HTMLElement) {
				setMeter(el, percent, variant);
			}
		}
	}

	if (map.badges) {
		for (const [key, value] of Object.entries(map.badges)) {
			const selector = `[data-badge="${key}"]`;
			const el = query(selector);
			if (el instanceof HTMLElement && value.variant) {
				setBadge(el, value.variant, value.label, value.size ?? "xxs");
			}
		}
	}

	if (map.vars) {
		for (const [key, value] of Object.entries(map.vars)) {
			// First check if an element with data-axis or data-var matches
			const axisEl = query(`[data-axis="${key}"]`);
			if (axisEl instanceof HTMLElement || axisEl instanceof SVGElement) {
				axisEl.style.setProperty("--axis", String(value));
				continue;
			}

			const varEl = query(`[data-var="${key}"]`);
			if (varEl instanceof HTMLElement || varEl instanceof SVGElement) {
				varEl.style.setProperty(`--${key}`, String(value));
				continue;
			}

			// Otherwise apply to root
			root.style.setProperty(`--${key}`, String(value));
		}
	}

	if (map.custom) {
		map.custom(root);
	}
};

/**
 * React hook that connects a TanStack Store (or any subscribable store) to direct DOM painting.
 * Runs synchronously on mount with initial state, subscribes to subsequent changes, and
 * automatically cleans up the subscription on unmount.
 */
export function usePaintStore<
	TState,
	TElement extends HTMLElement = HTMLDivElement,
>(
	store: SubscribableStore<TState> | undefined | null,
	mapper: (state: TState, root: TElement) => PaintMap | void,
	deps: DependencyList = [],
	forwardedRef?: RefObject<TElement | null> | null,
): RefObject<TElement | null> {
	const localRef = useRef<TElement | null>(null);
	const targetRef = forwardedRef ?? localRef;

	useEffect(() => {
		if (!store) return;
		const root = targetRef.current;
		if (!root) return;

		const cache = new Map<string, Element | null>();

		const update = (state: TState) => {
			const currentRoot = targetRef.current;
			if (!currentRoot) return;
			const paintMap = mapper(state, currentRoot);
			if (paintMap) {
				applyPaintMap(currentRoot, paintMap, cache);
			}
		};

		// 1. Apply initial store state immediately
		const initial =
			store.state !== undefined ? store.state : store.get?.();
		if (initial !== undefined) {
			update(initial as TState);
		}

		// 2. Subscribe to store changes
		const subscription = store.subscribe(update);

		return () => {
			if (typeof subscription === "function") {
				subscription();
			} else if (
				subscription &&
				typeof subscription.unsubscribe === "function"
			) {
				subscription.unsubscribe();
			}
		};
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [store, ...deps]);

	return targetRef;
}
