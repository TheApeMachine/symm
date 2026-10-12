import type { Store } from "@tanstack/store";
import { useEffect, useState } from "react";
import type { RingBuffer } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

/*
Plain readers over a Measurement's Metrics. They never invent a value: a
Metric the Measurement does not carry reads as undefined.
*/

const nameOf = (raw: string | Uint8Array | null | undefined): string =>
	!raw ? "" : typeof raw === "string" ? raw : new TextDecoder().decode(raw);

export const metricOf = (
	measurement: MeasurementT | undefined,
	name: string,
): number | undefined => {
	for (const metric of measurement?.metrics ?? []) {
		if (nameOf(metric?.name) === name && Number.isFinite(metric.raw)) {
			return metric.raw;
		}
	}

	return undefined;
};

/* metricsWithPrefix returns every Metric whose name starts with prefix, keyed by the remainder. */
export const metricsWithPrefix = (
	measurement: MeasurementT | undefined,
	prefix: string,
): Map<string, number> => {
	const out = new Map<string, number>();

	for (const metric of measurement?.metrics ?? []) {
		const name = nameOf(metric?.name);
		if (name.startsWith(prefix) && Number.isFinite(metric.raw)) {
			out.set(name.slice(prefix.length), metric.raw);
		}
	}

	return out;
};

export type MeasurementStore = Store<Record<string, RingBuffer<MeasurementT>>>;

/*
useMeasurements follows one symbol's ring in a store and returns the latest
Measurement plus up to `keep` of the most recent ones (oldest first), for
visuals that draw history. Measurements are only those the backend sent.
*/
export const useMeasurements = (
	store: MeasurementStore,
	symbol: string,
	keep = 120,
	accept?: (measurement: MeasurementT) => boolean,
): MeasurementT[] => {
	const [history, setHistory] = useState<MeasurementT[]>([]);

	useEffect(() => {
		const cursor = new RingCursor<MeasurementT>();
		let buffer: MeasurementT[] = [];

		const read = () => {
			const ring = store.state[symbol];
			if (!ring) return;
			let changed = false;

			cursor.read(ring, (measurement) => {
				if (accept && !accept(measurement)) return;
				buffer.push(measurement);
				changed = true;
			});

			if (!changed) return;
			if (buffer.length > keep) buffer = buffer.slice(-keep);
			setHistory(buffer.slice());
		};

		read();
		const subscription = store.subscribe(read);
		return () => subscription.unsubscribe();
	}, [store, symbol, keep, accept]);

	return history;
};
