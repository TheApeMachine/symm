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
	// Scalar layers arrive as a metric-keyed map.
	const metrics = item.metrics();
	const keys = Object.keys(metrics);

	if (name) {
		if (!keys.includes(name)) return null;
		return finiteSample(metrics[name]);
	}

	if (keys.length === 0) return null;
	return finiteSample(metrics[keys[0] ?? ""]);
};

/*
The sparkline tracks the series stored on the element: each frame appends its
sample and the window is capped, so the trend survives without a ring store.
*/
const appendSeries = (
	root: HTMLElement,
	frame: Value | undefined,
): void => {
	const svg = memoizedQuery<SVGElement>(root, '[data-k="sparkline"]');
	if (!svg) return;

	const name = svg.getAttribute("data-series");
	const sample = frame ? sampleMetric(frame, name) : null;

	const key = name || "";
	const series: number[] =
		(seriesStore.get(root)?.[key] ?? []).concat(
			sample === null ? [] : [sample],
		);
	const window = series.slice(-SERIES_WINDOW);

	const stored = seriesStore.get(root) ?? {};
	stored[key] = window;
	seriesStore.set(root, stored);

	if (window.length === 0) {
		setSparkline(svg, [], false);
		return;
	}

	setSparkline(svg, scaleSparklineValues(window), true);
};

const SERIES_WINDOW = 120;
const seriesStore = new WeakMap<HTMLElement, Record<string, number[]>>();

export const reset = (root: HTMLElement, fallback = "—") => {
	const elements = memoizedQueryAll(
		root,
		"[data-metric], [data-metric-raw], [data-metric-snm], [data-metric-meter], [data-metric-badge], [data-mbar]",
	);
	for (const el of elements) {
		resetValue(el, fallback);
	}
	seriesStore.delete(root);
	const svg = memoizedQuery<SVGElement>(root, '[data-k="sparkline"]');
	if (svg) setSparkline(svg, [], false);
};

/*
walk binds one frame to matching DOM elements. The frame's own fields
(scope, origin, timestamp) bind to their data-metric selectors — scope
also answering data-metric="symbol" — and each scalar layer of the frame
(the Go Append calls addressed below the prefix) binds by its metric name.
*/
export const walkFrame = (root: HTMLElement, frame: Value) => {
	const fields: Record<string, unknown> = {};
	for (const key of [
		"id",
		"epoch",
		"tick",
		"seqIdx",
		"timestamp",
		"origin",
		"role",
		"scope",
	] as const) {
		fields[key] = frame[key];
	}

	bind(root, fields);

	const metrics: Record<string, unknown> = {};
	for (const { suffix, payload } of frame.readings()) {
		if (suffix === "") continue;
		const first = Array.isArray(payload) ? payload[0] : payload;
		if (first === null || first === undefined) continue;
		metrics[suffix] = first;
	}

	bind(root, metrics);
};

/*
walk an object recursively and bind its fields directly to matching DOM elements:
- Array items with `name` bind to [data-metric="<name>"], [data-metric-raw="<name>"],
  [data-metric-snm="<name>"], [data-metric-meter="<name>"], [data-metric-badge="<name>"], [data-mbar="<name>"]
- Primitive properties `key: value` bind to [data-metric="<key>"],
  [data-metric-meter="<key>"], [data-metric-badge="<key>"]
- Nested objects and arrays are traversed recursively
*/
export const bind = (root: HTMLElement, obj: unknown) => {
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
					bind(root, item);
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
			bind(root, value);
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
	/* A signal store; with symbol, the Component follows that symbol's frame live. */
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
It subscribes to a Value or a named signal store
and dynamically binds incoming Value fields to matching data attributes in
its children DOM tree (e.g., data-metric="<metric_name>"), bypassing React's
render cycle for maximum streaming throughput without repetitive boilerplate.
*/
export const Component = ({
	value: propValue,
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
				walkFrame(root, propValue);
				if (onValue) onValue(propValue, root);
			} else {
				reset(root);
			}
			return;
		}

		// 2. Store subscription: the store holds one Value per scope.
		reset(root);

		const update = (frame: Value | undefined) => {
			if (!frame) {
				reset(root);
				return;
			}

			walkFrame(root, frame);
			if (onValue) onValue(frame, root);
			appendSeries(root, frame);
		};

		const resolve = () =>
			store && symbol !== undefined ? store.state[symbol] : undefined;

		const current = resolve();
		if (current) update(current);
		if (!store) return;

		const subscription = store.subscribe(() => {
			update(resolve());
		});

		return () => subscription.unsubscribe();
	}, [propValue, store, symbol, onValue]);

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
