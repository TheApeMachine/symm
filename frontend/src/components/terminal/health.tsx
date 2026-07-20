import { createRef } from "react";
import type { DashboardFrame, TickFrame } from "#/collections/types";
import type { Measurement } from "#/types/measurement";
import {
	backendMeasurementSources,
	sourceHasUniverseFrames,
} from "#/components/terminal/measurement-sources";
import {
	headlineReading,
	percentOf,
	resolveKernelStatus,
} from "#/components/terminal/measurement-view";
import { paintInlineMeter } from "#/components/terminal/metric-paint";
import { requirePositive } from "#/lib/domain";
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

/*
measurementTickCount reports how many observation timestamps a flat buffer
retains.
*/
const measurementTickCount = (rows: Measurement[]): number => {
	if (rows.length === 0) {
		return 0;
	}

	return new Set(rows.map((row) => row.at)).size;
};

export const terminalHealthSummary = (
	measurements: Measurement[],
	focusSymbol: string,
	sources = backendMeasurementSources(measurements),
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
		const sourceRows = measurements.filter((row) => row.source === source);
		const history =
			focusSymbol === "stream"
				? sourceRows
				: sourceRows.filter((row) => row.symbol === focusSymbol);
		const latest = headlineReading(history, source);
		const status =
			focusSymbol === "stream"
				? resolveKernelStatus(latest, history.length > 0)
				: resolveKernelStatus(
						latest,
						sourceHasUniverseFrames(measurements, source),
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
				: measurementTickCount(history) > 0 ||
					sourceHasUniverseFrames(measurements, source)
		) {
			firing += 1;
		}
	}

	const engine = engineHealthLabel(tick, { degraded, measured, total });
	const bars = [
		{ label: "Healthy", count: measured, variant: "success" as const },
		{ label: "Warming", count: warming, variant: "warning" as const },
		{ label: "Degraded", count: degraded, variant: "error" as const },
	].map((bar) => {
		if (total <= 0) {
			return { ...bar, percent: 0 };
		}

		const mass = requirePositive(total, "health bar mass");

		return {
			...bar,
			percent: Math.round((bar.count / mass) * 100),
		};
	});
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

const badgeRef = createRef<HTMLSpanElement>();
const healthyRef = createRef<HTMLDivElement>();
const avgRef = createRef<HTMLDivElement>();
const firingRef = createRef<HTMLDivElement>();
const tickRef = createRef<HTMLDivElement>();
const healthyMeterRef = createRef<HTMLDivElement>();
const warmingMeterRef = createRef<HTMLDivElement>();
const degradedMeterRef = createRef<HTMLDivElement>();

let lastMeasurements: Measurement[] = [];
let lastTick: TickFrame | null = null;
let lastFocusSymbol = "";

const paintStat = (
	node: HTMLDivElement | null,
	value: string,
	variant?: Variant,
) => {
	if (node === null) {
		return;
	}

	const valueNode = node.querySelector<HTMLElement>("[data-stat-value='true']");

	if (valueNode === null) {
		return;
	}

	valueNode.textContent = value;

	if (variant !== undefined) {
		valueNode.style.color = VARIANT_TONE[variant];
	}
};

const paintHealth = () => {
	const health = terminalHealthSummary(
		lastMeasurements,
		lastFocusSymbol,
		undefined,
		lastTick,
	);

	if (badgeRef.current !== null) {
		badgeRef.current.textContent = health.label;
		badgeRef.current.style.color = VARIANT_TONE[health.variant];
		badgeRef.current.style.background = `color-mix(in srgb, ${VARIANT_TONE[health.variant]} 12%, transparent)`;
		badgeRef.current.style.borderColor = `color-mix(in srgb, ${VARIANT_TONE[health.variant]} 38%, transparent)`;
	}

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
};

/*
paintHealthMeasurements refreshes system health from the current DRAW
measurements batch and the cached tick.
*/
export const paintHealthMeasurements = (
	value: unknown,
	focusSymbol: string,
) => {
	lastMeasurements = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Measurement[];
	lastFocusSymbol = focusSymbol;
	paintHealth();
};

/*
paintHealthTick refreshes system health from the current DRAW tick and the
cached measurements.
*/
export const paintHealthTick = (value: unknown, focusSymbol: string) => {
	const rows = Array.isArray(value) ? value : value != null ? [value] : [];
	lastTick = (rows.at(-1) as TickFrame | undefined) ?? null;
	lastFocusSymbol = focusSymbol;
	paintHealth();
};

/*
HealthPanel is the static system-health shell. DRAW paints via
paintHealthMeasurements and paintHealthTick.
*/
export const HealthPanel = () => (
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
