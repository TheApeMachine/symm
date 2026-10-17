import type { RingBuffer } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import type { Value } from "#/types/value";
import { type ReactNode, useEffect, useRef } from "react";
import { Alert } from "./alert";
import { memoizedQuery, memoizedQueryAll, renderValue, resetValue } from "./dom";
import { Loader } from "./loader";
import { scaleSparklineValues, setSparkline } from "./sparkline";
import type { ValueStore } from "./types";

export const nameOf = (val: unknown): string => {
	if (typeof val === "string") return val;
	if (val instanceof Uint8Array) return new TextDecoder().decode(val);
	return "";
};

/*
reset restores all bound telemetry and metric elements in root to their fallback state.
*/
const finiteSample = (value: number | undefined): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const sampleMetric = (item: Value, name: string | null): number | null => {
	if (name) {
		for (const metric of item.metrics ?? []) {
			if (metric?.name !== name) continue;
			return finiteSample(metric.snm) ?? finiteSample(metric.raw);
		}
	}

	const first = item.metrics?.[0];
	if (!first) return null;
	return finiteSample(first.snm) ?? finiteSample(first.raw);
};

const seriesFromRing = (
	ring: RingBuffer<Value>,
	name: string | null,
): number[] => {
	const points: number[] = [];
	const len = ring.getBufferLength();
	for (let i = 0; i < len; i++) {
		const item = ring.get(i);
		if (!item) continue;
		const sample = sampleMetric(item, name);
		if (sample !== null) points.push(sample);
	}
	return points;
};

const paintSparkline = (root: HTMLElement, ring?: RingBuffer<Value>) => {
	const svg = memoizedQuery<SVGElement>(root, '[data-k="sparkline"]');
	if (!svg) return;

	if (!ring || ring.isEmpty()) {
		setSparkline(svg, [], false);
		return;
	}

	const series = seriesFromRing(ring, svg.getAttribute("data-series"));
	setSparkline(svg, scaleSparklineValues(series), series.length > 0);
};

export const reset = (root: HTMLElement, fallback = "—") => {
	const elements = memoizedQueryAll(
		root,
		"[data-metric], [data-metric-raw], [data-metric-snm], [data-metric-meter], [data-metric-badge], [data-mbar]",
	);
	for (const el of elements) {
		resetValue(el, fallback);
	}
	paintSparkline(root);
};

/*
walk an object recursively and bind its fields directly to matching DOM elements:
- Array items with `name` bind to [data-metric="<name>"], [data-metric-raw="<name>"],
  [data-metric-snm="<name>"], [data-metric-meter="<name>"], [data-metric-badge="<name>"], [data-mbar="<name>"]
- Primitive properties `key: value` bind to [data-metric="<key>"],
  [data-metric-meter="<key>"], [data-metric-badge="<key>"]
- Nested objects and arrays are traversed recursively
*/
export const walk = (root: HTMLElement, obj: unknown) => {
	if (obj === null || obj === undefined || typeof obj !== "object") return;

	for (const [key, value] of Object.entries(obj)) {
		if (value === null || value === undefined) continue;

		if (Array.isArray(value)) {
			for (const item of value) {
				if (item === null || item === undefined) continue;

				if (typeof item === "object") {
					const name = "name" in item ? nameOf(item.name) : "";
					if (name) {
						renderValue(
							memoizedQuery(root, `[data-metric="${name}"]`),
							item,
						);
						if ("raw" in item) {
							renderValue(
								memoizedQuery(
									root,
									`[data-metric-raw="${name}"], [data-f="m:${name}"]`,
								),
								item.raw,
							);
						}
						if ("snm" in item) {
							renderValue(
								memoizedQuery(root, `[data-metric-snm="${name}"]`),
								item.snm,
							);
						}
						renderValue(
							memoizedQuery(
								root,
								`[data-metric-meter="${name}"], [data-mbar="${name}"]`,
							),
							item,
						);
						renderValue(
							memoizedQuery(root, `[data-metric-badge="${name}"]`),
							item,
						);
					}
					walk(root, item);
				} else {
					renderValue(
						memoizedQuery(
							root,
							`[data-metric="${key}"], [data-metric-meter="${key}"], [data-metric-badge="${key}"]`,
						),
						item,
					);
				}
			}
			continue;
		}

		if (typeof value === "object") {
			walk(root, value);
			continue;
		}

		const resolved =
			typeof value === "string" || value instanceof Uint8Array
				? nameOf(value) || value
				: value;

		const selectors = [
			`[data-metric="${key}"]`,
			`[data-metric-meter="${key}"]`,
			`[data-metric-badge="${key}"]`,
		];
		if (key === "scope") {
			selectors.push(
				'[data-metric="symbol"]',
				'[data-metric-meter="symbol"]',
				'[data-metric-badge="symbol"]',
			);
		} else if (key === "origin") {
			selectors.push(
				'[data-metric="source"]',
				'[data-metric-meter="source"]',
				'[data-metric-badge="source"]',
			);
		} else if (key === "timestamp") {
			selectors.push(
				'[data-metric="at"]',
				'[data-metric-meter="at"]',
				'[data-metric-badge="at"]',
			);
		}

		renderValue(memoizedQuery(root, selectors.join(", ")), resolved);
	}
};

export interface ComponentProps {
	/* Direct single value to bind */
	value?: Value | null;
	/* Direct ring buffer to observe */
	ringBuffer?: RingBuffer<Value>;
	/* A signal store; with symbol, the Component follows that symbol's ring live. */
	store?: ValueStore;
	source?: string;
	symbol?: string;
	children?: ReactNode;
	className?: string;
	onValue?: (value: Value, root: HTMLElement) => void;
	isLoading?: boolean;
	error?: Error;
}

/*
Component acts as a dynamic telemetry data-binding boundary.
It subscribes to a Value, RingBuffer<Value>, or a named signal store
and dynamically binds incoming Value fields to matching data attributes in
its children DOM tree (e.g., data-metric="<metric_name>"), bypassing React's
render cycle for maximum streaming throughput without repetitive boilerplate.
*/
export const Component = ({
	value: propValue,
	ringBuffer: propRing,
	store,
	symbol,
	children,
	className,
	onValue,
	isLoading,
	error,
}: ComponentProps) => {
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const root = containerRef.current;
		if (!root) return;

		// 1. Direct value prop
		if (propValue !== undefined) {
			if (propValue) {
				walk(root, propValue);
				if (onValue) onValue(propValue, root);
			} else {
				reset(root);
			}
			return;
		}

		// 2. RingBuffer or Store subscription
		reset(root);

		const cursor = new RingCursor<Value>();
		let observedRing: RingBuffer<Value> | undefined;
		let generation = -1;

		const update = (ring: RingBuffer<Value> | undefined) => {
			if (!ring || ring.isEmpty()) {
				reset(root);
				return;
			}

			if (observedRing !== ring || generation !== ring.generation) {
				observedRing = ring;
				generation = ring.generation;
			}

			cursor.read(ring, (val) => {
				walk(root, val);
				if (onValue) onValue(val, root);
			});
			paintSparkline(root, ring);
		};

		const resolve = () =>
			propRing ??
			(store && symbol !== undefined ? store.state[symbol] : undefined);

		const current = resolve();
		if (current) update(current);
		if (!store) return;

		const subscription = store.subscribe(() => {
			const ring = resolve();
			update(ring);
		});

		return () => subscription.unsubscribe();
	}, [propValue, propRing, store, symbol, onValue]);

	if (isLoading) {
		return <Loader />;
	}

	if (error) {
		return <Alert>{error.message}</Alert>;
	}

	return (
		<div ref={containerRef} className={className}>
			{children}
		</div>
	);
};
