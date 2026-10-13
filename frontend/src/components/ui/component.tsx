import type { RingBuffer } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { type ReactNode, useEffect, useRef } from "react";
import { Alert } from "./alert";
import { memoizedQuery, memoizedQueryAll, renderValue, resetValue } from "./dom";
import { Loader } from "./loader";
import type { MeasurementStore } from "./measurement";

export const nameOf = (val: unknown): string => {
	if (typeof val === "string") return val;
	if (val instanceof Uint8Array) return new TextDecoder().decode(val);
	return "";
};

/*
reset restores all bound telemetry and metric elements in root to their fallback state.
*/
export const reset = (root: HTMLElement, fallback = "—") => {
	const elements = memoizedQueryAll(
		root,
		"[data-metric], [data-metric-raw], [data-metric-snm], [data-metric-meter], [data-metric-badge], [data-mbar]",
	);
	for (const el of elements) {
		resetValue(el, fallback);
	}
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

		renderValue(
			memoizedQuery(
				root,
				`[data-metric="${key}"], [data-metric-meter="${key}"], [data-metric-badge="${key}"]`,
			),
			resolved,
		);
	}
};

export interface ComponentProps {
	/* Direct single measurement to bind */
	measurement?: MeasurementT | null;
	/* Direct ring buffer to observe */
	ringBuffer?: RingBuffer<MeasurementT>;
	/* A signal store; with symbol, the Component follows that symbol's ring live. */
	store?: MeasurementStore;
	source?: string;
	symbol?: string;
	children?: ReactNode;
	className?: string;
	onMeasurement?: (measurement: MeasurementT, root: HTMLElement) => void;
	isLoading?: boolean;
	error?: Error;
}

/*
Component acts as a dynamic telemetry data-binding boundary.
It subscribes to a MeasurementT, RingBuffer<MeasurementT>, or a named signal store
and dynamically binds incoming Measurement fields to matching data attributes in
its children DOM tree (e.g., data-metric="<metric_name>"), bypassing React's
render cycle for maximum streaming throughput without repetitive boilerplate.
*/
export const Component = ({
	measurement: propMeasurement,
	ringBuffer: propRing,
	store,
	symbol,
	children,
	className,
	onMeasurement,
	isLoading,
	error,
}: ComponentProps) => {
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const root = containerRef.current;
		if (!root) return;

		// 1. Direct measurement prop
		if (propMeasurement !== undefined) {
			if (propMeasurement) {
				walk(root, propMeasurement);
				if (onMeasurement) onMeasurement(propMeasurement, root);
			} else {
				reset(root);
			}
			return;
		}

		// 2. RingBuffer or Store subscription
		reset(root);

		const cursor = new RingCursor<MeasurementT>();
		let observedRing: RingBuffer<MeasurementT> | undefined;
		let generation = -1;

		const update = (ring: RingBuffer<MeasurementT> | undefined) => {
			if (!ring || ring.isEmpty()) {
				reset(root);
				return;
			}

			if (observedRing !== ring || generation !== ring.generation) {
				observedRing = ring;
				generation = ring.generation;
			}

			cursor.read(ring, (measurement) => {
				walk(root, measurement);
				if (onMeasurement) onMeasurement(measurement, root);
			});
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
	}, [propMeasurement, propRing, store, symbol, onMeasurement]);

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
