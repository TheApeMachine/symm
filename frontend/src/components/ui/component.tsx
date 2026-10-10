import {
	type ReactNode,
	useEffect,
	useRef,
} from "react";
import type { RingBuffer } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import {
	memoizedQuery,
	renderValue,
} from "./dom";
import { Loader } from "./loader";
import { Alert } from "./alert";

/*
walk the object and bind its values to the DOM.
*/
const walk = (root: HTMLElement, measurement: MeasurementT) => {
	Object.entries(measurement).forEach(([key, value]) => {
		if (value === null || value === undefined) return;

		if (Array.isArray(value)) {
			value.forEach((item) => {
				if (item === null || item === undefined) return;
				if (typeof item === "object") {
					walk(root, item);
				} else {
					renderValue(memoizedQuery(root, `[data-metric="${key}"]`), item);
				}
			});

			return;
		}

		if (typeof value === "object") {
			walk(root, value);
			return;
		}

		renderValue(memoizedQuery(root, `[data-metric="${key}"]`), value);
	});
};

export interface ComponentProps {
	ringBuffer?: RingBuffer<MeasurementT>;
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
It subscribes to a RingBuffer<MeasurementT> (or a named signal source) and
dynamically binds incoming Measurement fields to matching data attributes in
its children DOM tree (e.g., data-metric="<metric_name>"), bypassing React's
render cycle for maximum streaming throughput without repetitive boilerplate.
*/
export const Component = ({
	ringBuffer: propRing,
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

		const cursor = new RingCursor<MeasurementT>();
		const sparklineHistory: number[] = [];
		let observedRing: RingBuffer<MeasurementT> | undefined;
		let generation = -1;

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (observedRing !== ring || generation !== ring.generation) {
				sparklineHistory.length = 0;
				observedRing = ring;
				generation = ring.generation;
			}

			if (ring && !ring.isEmpty()) {
                cursor.read(ring, (measurement) => {
					walk(root, measurement);
					if (onMeasurement) onMeasurement(measurement, root);
				});
            }
		};

		if (propRing) update(propRing);
	}, [propRing, onMeasurement]);

    if (isLoading) {
        return <Loader />;
    }

    if (error) {
        return (
            <Alert>{error.message}</Alert>
        );
    }

	return (
		<div
			ref={containerRef}
			className={className}
		>
			{children}
		</div>
	);
};
