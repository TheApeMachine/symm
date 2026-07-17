import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import type { DashboardFrame } from "#/collections/frames";
import {
	flattenMeasurementBuffer,
	measurementsStore,
	measurementTickCount,
} from "#/collections/measurements";
import { tickStore } from "#/collections/tick";
import { sourceHasUniverseFrames } from "#/components/terminal/measurement-sources";
import {
	headlineReading,
	percentOf,
	resolveKernelStatus,
} from "#/components/terminal/measurement-view";
import { paintInlineMeter } from "#/components/terminal/metric-paint";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { Badge } from "@/components/ui/badge";
import { Flex } from "@/components/ui/flex";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";
import type { Variant } from "@/components/ui/types";

export type HealthSummary = {
	bars: Array<{
		variant: Variant;
		count: number;
		label: string;
		percent: number;
	}>;
	avg: number;
	firing: number;
	label: string;
	measured: number;
	variant: Variant;
	total: number;
	tickMs: number | null;
	completed: boolean;
};

/*
engineHealthLabel prefers completed planner ticks over focus-scoped kernel
standby so a silent BTC focus cannot paint the engine as dead while ticks run.
*/
/*
tickCompleted prefers an explicit completed flag (including false) and only
infers completion from a numeric count when the flag is absent.
*/
export const tickCompleted = (tick: DashboardFrame | null): boolean =>
	typeof tick?.completed === "boolean"
		? tick.completed
		: typeof tick?.count === "number";

export const engineHealthLabel = (
	tick: DashboardFrame | null,
	focus: { degraded: number; measured: number; total: number },
): { label: string; variant: Variant } => {
	const completed = tickCompleted(tick);

	if (!completed) {
		return { label: "Silent", variant: "warning" };
	}

	if (focus.degraded > 0) {
		return { label: "Degraded", variant: "error" };
	}

	if (focus.total > 0 && focus.measured < focus.total / 2) {
		return { label: "Live · thin focus", variant: "warning" };
	}

	return { label: "Live", variant: "success" };
};

export const terminalHealthSummary = (
	readings: typeof measurementsStore.state,
	focusSymbol: string,
	sources = [
		...new Set(
			Object.values(readings.measurements).flatMap((sourceMap) =>
				Object.keys(sourceMap),
			),
		),
	],
	tick: DashboardFrame | null = null,
): HealthSummary => {
	const total = sources.length;
	let measured = 0;
	let warming = 0;
	let degraded = 0;
	let strength = 0;
	let strengthCount = 0;
	let firing = 0;

	for (const source of sources) {
		const buffer =
			focusSymbol === "stream"
				? undefined
				: readings.measurements[focusSymbol]?.[source];
		const history =
			focusSymbol === "stream"
				? Object.values(readings.measurements).flatMap((sourceMap) =>
						flattenMeasurementBuffer(sourceMap[source]),
					)
				: flattenMeasurementBuffer(buffer);
		const latest = headlineReading(history, source);
		const status =
			focusSymbol === "stream"
				? resolveKernelStatus(latest, history.length > 0)
				: resolveKernelStatus(
						latest,
						sourceHasUniverseFrames(readings, source),
					);

		if (status === "fault") {
			degraded += 1;
		} else if (
			status === "waiting" ||
			status === "calibrating" ||
			status === "unfocused"
		) {
			warming += 1;
		} else {
			measured += 1;
		}

		if (latest !== undefined) {
			strength += percentOf(latest) / 100;
			strengthCount += 1;
		}

		if (
			focusSymbol === "stream"
				? history.length > 0
				: measurementTickCount(buffer) > 0 ||
					sourceHasUniverseFrames(readings, source)
		) {
			firing += 1;
		}
	}

	const engine = engineHealthLabel(tick, { degraded, measured, total });
	const bars = [
		{ label: "Healthy", count: measured, variant: "success" as const },
		{ label: "Warming", count: warming, variant: "warning" as const },
		{ label: "Degraded", count: degraded, variant: "error" as const },
	].map((bar) => ({
		...bar,
		percent: total > 0 ? Math.round((bar.count / total) * 100) : 0,
	}));
	const ns = typeof tick?.ns === "number" ? tick.ns : null;

	return {
		avg: strengthCount > 0 ? Math.round((strength / strengthCount) * 100) : 0,
		bars,
		firing,
		label: engine.label,
		measured,
		variant: engine.variant,
		total,
		tickMs: ns === null ? null : Math.round(ns / 1_000_000),
		completed: tickCompleted(tick),
	};
};

const VARIANT_TONE: Record<Variant, string> = {
	brand: "var(--brand)",
	info: "var(--info)",
	success: "var(--success)",
	warning: "var(--warning)",
	error: "var(--error)",
	disabled: "var(--f3)",
};

/*
HealthPanel paints system-health readouts directly from the measurement store.
*/
export const HealthPanel = ({ sources }: { sources?: string[] }) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const badgeRef = useRef<HTMLSpanElement>(null);
	const healthyRef = useRef<HTMLDivElement>(null);
	const avgRef = useRef<HTMLDivElement>(null);
	const firingRef = useRef<HTMLDivElement>(null);
	const tickRef = useRef<HTMLDivElement>(null);
	const healthyMeterRef = useRef<HTMLDivElement>(null);
	const warmingMeterRef = useRef<HTMLDivElement>(null);
	const degradedMeterRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const health = terminalHealthSummary(
				measurementsStore.state,
				focusSymbol,
				sources,
				tickStore.state.frame,
			);

			if (badgeRef.current !== null) {
				badgeRef.current.textContent = health.label;
				badgeRef.current.style.color = VARIANT_TONE[health.variant];
				badgeRef.current.style.background = `color-mix(in srgb, ${VARIANT_TONE[health.variant]} 12%, transparent)`;
				badgeRef.current.style.borderColor = `color-mix(in srgb, ${VARIANT_TONE[health.variant]} 38%, transparent)`;
			}

			const paintStat = (
				node: HTMLDivElement | null,
				value: string,
				variant?: Variant,
			) => {
				if (node === null) {
					return;
				}

				const valueNode = node.querySelector<HTMLElement>(
					"[data-stat-value='true']",
				);

				if (valueNode === null) {
					return;
				}

				valueNode.textContent = value;

				if (variant !== undefined) {
					valueNode.style.color = VARIANT_TONE[variant];
				}
			};

			paintStat(healthyRef.current, `${health.measured}/${health.total}`);
			paintStat(avgRef.current, `${health.avg}%`);
			paintStat(firingRef.current, String(health.firing), "warning");
			paintStat(
				tickRef.current,
				health.tickMs === null ? "—" : `${health.tickMs}ms`,
				health.completed ? "success" : "warning",
			);

			const [healthyBar, warmingBar, degradedBar] = health.bars;

			if (healthyBar !== undefined && healthyMeterRef.current !== null) {
				paintInlineMeter(
					healthyMeterRef.current,
					healthyBar.label,
					String(healthyBar.count),
					healthyBar.percent,
					healthyBar.variant as "success" | "warning" | "error",
				);
			}

			if (warmingBar !== undefined && warmingMeterRef.current !== null) {
				paintInlineMeter(
					warmingMeterRef.current,
					warmingBar.label,
					String(warmingBar.count),
					warmingBar.percent,
					warmingBar.variant as "success" | "warning" | "error",
				);
			}

			if (degradedBar !== undefined && degradedMeterRef.current !== null) {
				paintInlineMeter(
					degradedMeterRef.current,
					degradedBar.label,
					String(degradedBar.count),
					degradedBar.percent,
					degradedBar.variant as "success" | "warning" | "error",
				);
			}
		},
		[measurementsStore, tickStore],
		[focusSymbol, sources],
	);

	return (
		<Panel size="lg">
			<Flex.Row align="center" justify="between">
				<Flex className="font-semibold text-(--f1) text-xs">System health</Flex>
				<Badge
					ref={badgeRef}
					label=""
					className="rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
				/>
			</Flex.Row>
			<Flex.Row className="mt-3 gap-[18px]">
				<Stat
					ref={tickRef}
					value=""
					label="tick"
					emphasis="default"
					valueClassName="font-normal"
				/>
				<Stat
					ref={healthyRef}
					value=""
					label="healthy"
					emphasis="default"
					valueClassName="font-normal text-(--f1)"
				/>
				<Stat
					ref={avgRef}
					value=""
					label="avg strength"
					emphasis="default"
					valueClassName="font-normal text-(--f1)"
				/>
				<Stat
					ref={firingRef}
					value=""
					label="firing"
					emphasis="default"
					valueClassName="font-normal"
				/>
			</Flex.Row>
			<Flex.Column className="mt-[13px] gap-1.5">
				<Meter
					ref={healthyMeterRef}
					layout="inline"
					size="xs"
					percent={0}
					label=""
					value=""
				/>
				<Meter
					ref={warmingMeterRef}
					layout="inline"
					size="xs"
					percent={0}
					label=""
					value=""
				/>
				<Meter
					ref={degradedMeterRef}
					layout="inline"
					size="xs"
					percent={0}
					label=""
					value=""
				/>
			</Flex.Column>
		</Panel>
	);
};
