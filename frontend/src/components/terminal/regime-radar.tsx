import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import {  focusAtom , signals } from "#/collections/app";
import { Panel } from "#/components/ui/panel";
import { Radar } from "#/components/ui/radar";
import { applyPaintMap } from "#/components/ui/paint";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

const radarAxes = [
	{
		label: "volatility",
		source: "hawkes",
		metric: "spectral_radius",
		x: 0,
		y: -1,
	},
	{ label: "trend", source: "pumpdump", metric: "trend", x: 0.951, y: -0.309 },
	{ label: "drive", source: "cvd", metric: "drive", x: 0.588, y: 0.809 },
	{
		label: "starved",
		source: "cvd",
		metric: "starvation",
		x: -0.588,
		y: 0.809,
	},
	{ label: "chop", source: "cvd", metric: "balance", x: -0.951, y: -0.309 },
];

const readNormalizedMetric = (row: any, metricName: string): number => {
	if (!row) return 0;
	if (Array.isArray(row.metrics)) {
		for (const m of row.metrics) {
			if (m && m.name === metricName) {
				return m.normalized ?? 0;
			}
		}
	} else if (typeof row.metricsLength === "function") {
		for (let j = 0; j < row.metricsLength(); j++) {
			const m = row.metrics(j, metricObj);
			if (m && m.name() === metricName) {
				return m.normalized() ?? 0;
			}
		}
	}
	return 0;
};

export const RadarPanel = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const subscriptions = radarAxes.map((axis) => {
			const store = (signals[axis.source as keyof typeof signals] || signals.cvd);

			const apply = (state: any) => {
				if (!root.current || !state || typeof state.getLast !== "function") return;
				const normalized = readNormalizedMetric(state.getLast(), axis.metric);
				applyPaintMap(root.current, {
					vars: {
						[axis.label]: Math.min(1, Math.max(0, normalized)),
					},
				});
			};

			apply(store.state);
			return store.subscribe(apply);
		});

		return () => {
			for (const subscription of subscriptions) {
				subscription.unsubscribe();
			}
		};
	}, []);

	return (
		<Panel ref={root} size="lg" className="h-full">
			<Panel.Header title="Regime radar" />
			<Panel.Caption>{focusSymbol} · normalized axes</Panel.Caption>
			<Radar axes={radarAxes} />
		</Panel>
	);
};
