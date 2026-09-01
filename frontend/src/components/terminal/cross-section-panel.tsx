import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import type { FrameBuffer } from "#/collections/app";
import { focusStore, getMeasurementStore } from "#/collections/app";
import { Panel } from "#/components/ui/panel";
import type { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import { EnvelopeMeasurementMetric } from "#/providers/telemetry/telemetry/envelope-measurement-metric";

const metricObj = new EnvelopeMeasurementMetric();

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : "—";

const STATS = [
	{ label: "med notional", name: "reported_volume_notional_median", align: "text-left" },
	{ label: "med depth", name: "executable_touch_depth_median", align: "text-center" },
	{ label: "touch depth", name: "executable_touch_depth", align: "text-right" },
] as const;

export const CrossSectionPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const store = getMeasurementStore("liquidity", focusSymbol);
		const apply = (state: FrameBuffer<EnvelopeMeasurement>) => {
		if (!root.current) return;
		const row = state.getLast();

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
			if (el) el.textContent = value;
		};

		const metricsMap: Record<string, { raw: number; normalized: number }> = {};
		if (row) {
			for (let j = 0; j < row.metricsLength(); j++) {
				const m = row.metrics(j, metricObj);
				if (m) {
					metricsMap[m.key() ?? ""] = {
						raw: m.value()?.raw() ?? 0,
						normalized: m.value()?.normalized() ?? 0,
					};
				}
			}
		}

		set("scarcity", fmt(metricsMap.scarcity_score?.raw, 3));
		set("symbol", focusSymbol.length === 0 ? "no focus" : focusSymbol);
		set("rel", fmt(metricsMap.relative_touch_depth?.raw, 3));
		set("at", (() => {
			if (row?.atNs() === undefined) return "—";
			const parsed = new Date(Number(row.atNs() / 1000000n));
			return Number.isNaN(parsed.getTime()) ? "—" : parsed.toISOString().slice(11, 19);
		})());
		set("norm", fmt(metricsMap.executable_touch_depth?.normalized, 2));

		for (const stat of STATS) {
			set(stat.name, fmt(metricsMap[stat.name]?.raw, 0));
		}

		// depth bar
		const depth = metricsMap.executable_touch_depth?.raw;
		const median = metricsMap.executable_touch_depth_median?.raw;
		const bar = root.current.querySelector<HTMLElement>("[data-depth-bar]");
		const progressbar = root.current.querySelector<HTMLElement>('[role="progressbar"]');

		if (bar instanceof HTMLElement) {
			if (depth !== undefined && median !== undefined && median > 0) {
				const clamped = Math.min(100, Math.max(0, (depth / median) * 100));
				bar.style.width = `${clamped.toFixed(3)}%`;
				progressbar?.setAttribute("aria-valuenow", String(clamped));
			} else {
				bar.style.width = "0%";
				progressbar?.setAttribute("aria-valuenow", "0");
			}
		}
		};

		apply(store.state);
		const subscription = store.subscribe(apply);
		return () => subscription.unsubscribe();
	}, [focusSymbol]);

	return (
		<Panel ref={root} size="lg">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">Cross-section</span>
				<span data-f="scarcity" className="rounded-[3px] border border-(--line2) px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide">—</span>
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				liquidity axes · <span data-f="symbol" />
			</div>
			<div className="flex items-center justify-between">
				<span className="font-mono text-[11px] text-(--f2)">
					relative depth <span data-f="rel" className="text-(--acc)" />
				</span>
				<span data-f="at" className="font-mono text-[10px] text-(--f4)" />
			</div>
			<div className="mt-2.5">
				<div className="flex items-center gap-2" role="progressbar" aria-valuemin={0} aria-valuemax={100}>
					<span className="w-13.5 shrink-0 font-mono text-[9px] text-(--f4)">Depth</span>
					<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line) [--meter-tone:var(--info)]">
						<div data-depth-bar className="h-full bg-(--meter-tone)" style={{ width: "0%" }} />
					</div>
					<span data-f="norm" className="w-7 shrink-0 text-right font-mono text-[9px] text-(--f2)" />
				</div>
			</div>
			<div className="mt-3.25 flex justify-between gap-3">
				{STATS.map((stat) => (
					<div key={stat.name} className={`min-w-0 flex-1 ${stat.align}`}>
						<div data-f={stat.name} className="truncate font-mono text-lg text-(--f1) leading-none">—</div>
						<div className="mt-1 font-mono text-[9px] text-(--f4)">{stat.label}</div>
					</div>
				))}
			</div>
		</Panel>
	);
};

