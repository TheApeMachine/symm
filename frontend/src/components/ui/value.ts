import type { Value } from "#/types/value";
import { useEffect, useState } from "react";
import type { ValueStore } from "./types";

export type { ValueStore };

/*
useValues subscribes to a Value store and returns the most recent values for a symbol.
*/
export function useValues(
	store: ValueStore | undefined,
	symbol: string | undefined,
	depth = 60,
): Value[] {
	const [values, setValues] = useState<Value[]>(() => {
		if (!store || !symbol) return [];
		const ring = store.state[symbol];
		if (!ring) return [];
		const all = ring.toArray();
		return all.slice(Math.max(0, all.length - depth));
	});

	useEffect(() => {
		if (!store || !symbol) {
			setValues([]);
			return;
		}

		const update = () => {
			const ring = store.state[symbol];
			if (!ring) {
				setValues([]);
				return;
			}
			const all = ring.toArray();
			setValues(all.slice(Math.max(0, all.length - depth)));
		};

		update();
		const sub = store.subscribe(update);
		return () => {
			if (typeof sub === "function") {
				(sub as () => void)();
			} else if (sub && typeof (sub as { unsubscribe?: () => void }).unsubscribe === "function") {
				(sub as { unsubscribe: () => void }).unsubscribe();
			}
		};
	}, [store, symbol, depth]);

	return values;
}

/*
metricOf returns the numeric raw value of a named metric from a Value.
*/
export function metricOf(val: Value | undefined | null, name: string): number | undefined {
	if (!val) return undefined;
	if (typeof val.metric === "function") {
		const m = val.metric(name);
		if (m !== undefined) return m;
	}
	if (Array.isArray(val.metrics)) {
		for (const m of val.metrics) {
			if (m && m.name === name) return m.raw;
		}
	}
	return undefined;
}

/*
metricsWithPrefix returns a Map of metric names (stripped of prefix) to raw numeric values.
*/
export function metricsWithPrefix(
	val: Value | undefined | null,
	prefix: string,
): Map<string, number> {
	const out = new Map<string, number>();
	if (!val || !Array.isArray(val.metrics)) return out;
	for (const m of val.metrics) {
		if (m && typeof m.name === "string" && m.name.startsWith(prefix)) {
			out.set(m.name.slice(prefix.length), m.raw);
		}
	}
	return out;
}
