import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import {
	flattenMeasurementBuffer,
	measurementsStore,
	measurementTickCount,
} from "#/collections/measurements";
import {
	headlineMetric,
	latestByMetric,
	percentOf,
	resolveStatus,
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
		const headline = headlineMetric(source);
		const latest =
			headline === null ? history.at(-1) : latestByMetric(history, headline);
		const status = resolveStatus(latest);

		if (status === "fault") {
			degraded += 1;
		} else if (status === "waiting" || status === "calibrating") {
			warming += 1;
		} else {
			measured += 1;
		}

		if (headline !== null && latest !== undefined) {
			strength += percentOf(latest) / 100;
			strengthCount += 1;
		}

		if (
			focusSymbol === "stream"
				? history.length > 0
				: measurementTickCount(buffer) > 0
		) {
			firing += 1;
		}
	}

	const label =
		degraded > 0 ? "Degraded" : measured < total / 2 ? "Thin" : "Nominal";
	const variant: Variant =
		degraded > 0 ? "error" : measured < total / 2 ? "warning" : "success";
	const bars = [
		{ label: "Healthy", count: measured, variant: "success" as const },
		{ label: "Warming", count: warming, variant: "warning" as const },
		{ label: "Degraded", count: degraded, variant: "error" as const },
	].map((bar) => ({
		...bar,
		percent: total > 0 ? Math.round((bar.count / total) * 100) : 0,
	}));

	return {
		avg: strengthCount > 0 ? Math.round((strength / strengthCount) * 100) : 0,
		bars,
		firing,
		label,
		measured,
		variant,
		total,
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
	const healthyMeterRef = useRef<HTMLDivElement>(null);
	const warmingMeterRef = useRef<HTMLDivElement>(null);
	const degradedMeterRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const health = terminalHealthSummary(
				measurementsStore.state,
				focusSymbol,
				sources,
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
		[measurementsStore],
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

/*
regimeAxes projects typed numerical measurements onto the five fields in the
backend Regime contract, then averages each field across reporting symbols.
Turbulence supplies volatility, pump trend supplies trend, and signed CVD
pressure supplies bullish, bearish, and stochastic-balance evidence.
*/
export const regimeAxes = (
	readings: typeof measurementsStore.state,
	sources: string[],
): Array<{ label: string; value: number }> => {
	const available = new Set(sources);
	let volatile = 0;
	let volatileCount = 0;
	let trending = 0;
	let trendingCount = 0;
	let bullish = 0;
	let bearish = 0;
	let directionalCount = 0;
	let choppy = 0;
	let choppyCount = 0;

	for (const sourceMap of Object.values(readings.measurements)) {
		const turbulence = available.has("fluid")
			? latestByMetric(
					flattenMeasurementBuffer(sourceMap.fluid),
					"turbulent_score",
				)
			: undefined;

		if (turbulence !== undefined && turbulence.validity.state !== "invalid") {
			volatile += percentOf(turbulence) / 100;
			volatileCount += 1;
		}

		const trend = available.has("pumpdump")
			? latestByMetric(flattenMeasurementBuffer(sourceMap.pumpdump), "trend")
			: undefined;

		if (trend !== undefined && trend.validity.state !== "invalid") {
			trending += percentOf(trend) / 100;
			trendingCount += 1;
		}

		if (!available.has("cvd")) {
			continue;
		}

		const flow = flattenMeasurementBuffer(sourceMap.cvd);
		const net = latestByMetric(flow, "net");
		const netFraction = latestByMetric(flow, "net_fraction");
		const balance = latestByMetric(flow, "balance");

		if (
			net !== undefined &&
			netFraction !== undefined &&
			net.validity.state !== "invalid" &&
			netFraction.validity.state !== "invalid"
		) {
			const pressure = percentOf(netFraction) / 100;
			bullish += net.raw > 0 ? pressure : 0;
			bearish += net.raw < 0 ? pressure : 0;
			directionalCount += 1;
		}

		if (balance !== undefined && balance.validity.state !== "invalid") {
			choppy += percentOf(balance) / 100;
			choppyCount += 1;
		}
	}

	return [
		{
			label: "volatility",
			value: volatileCount > 0 ? volatile / volatileCount : 0,
		},
		{ label: "trend", value: trendingCount > 0 ? trending / trendingCount : 0 },
		{
			label: "bullish",
			value: directionalCount > 0 ? bullish / directionalCount : 0,
		},
		{
			label: "bearish",
			value: directionalCount > 0 ? bearish / directionalCount : 0,
		},
		{ label: "chop", value: choppyCount > 0 ? choppy / choppyCount : 0 },
	];
};

const RADAR_UNITS: Array<[number, number]> = [
	[0, -1],
	[0.951, -0.309],
	[0.588, 0.809],
	[-0.588, 0.809],
	[-0.951, -0.309],
];

/*
RadarPanel paints the five-axis market regime directly from typed measurement
snapshots so the right rail stays off the React render path.
*/
export const RadarPanel = ({ sources }: { sources?: string[] }) => {
	const radarFillRef = useRef<SVGPolygonElement>(null);
	const axisLabelRefs = useRef<Array<SVGTextElement | null>>([]);

	useDirectStorePaint(
		() => {
			const readings = measurementsStore.state;
			const candidates = sources ?? [
				...new Set(
					Object.values(readings.measurements).flatMap((sourceMap) =>
						Object.keys(sourceMap),
					),
				),
			];
			const axes = regimeAxes(readings, candidates);
			const points = RADAR_UNITS.map(
				([x, y], index) =>
					`${110 + x * 84 * (axes[index]?.value ?? 0)},${
						105 + y * 84 * (axes[index]?.value ?? 0)
					}`,
			).join(" ");

			if (radarFillRef.current !== null) {
				radarFillRef.current.setAttribute("points", points);
			}

			for (const [index, [x, y]] of RADAR_UNITS.entries()) {
				const label = axisLabelRefs.current[index];

				if (label === null || label === undefined) {
					continue;
				}

				label.textContent = axes[index]?.label ?? "—";
				label.setAttribute("x", String(110 + x * 98));
				label.setAttribute("y", String(105 + y * 98));
			}
		},
		[measurementsStore],
		[sources],
	);

	return (
		<Panel size="lg">
			<div className="mb-2 font-semibold text-(--f1) text-xs">Regime radar</div>
			<div className="mb-2 font-mono text-[9.5px] text-(--f4)">
				cross-section mean · market
			</div>
			<svg viewBox="0 0 220 210" className="block w-full">
				<title>Regime radar</title>
				<polygon
					points="110,21 190,79 159,173 61,173 30,79"
					fill="none"
					stroke="#3a342b"
				/>
				<polygon
					points="110,49 163,87 142,154 78,154 57,87"
					fill="none"
					stroke="#2b251e"
				/>
				<polygon
					points="110,77 137,94 126,134 94,134 83,94"
					fill="none"
					stroke="#2b251e"
				/>
				{RADAR_UNITS.map(([x, y]) => (
					<line
						key={`${x}:${y}`}
						x1="110"
						y1="105"
						x2={110 + x * 84}
						y2={105 + y * 84}
						stroke="#2b251e"
					/>
				))}
				<polygon
					ref={radarFillRef}
					fill="rgba(232,163,61,0.22)"
					stroke="#e8a33d"
					strokeWidth="1.6"
				/>
				{RADAR_UNITS.map(([x, y], index) => (
					<text
						key={`${x}:${y}`}
						ref={(element) => {
							axisLabelRefs.current[index] = element;
						}}
						x={110 + x * 98}
						y={105 + y * 98}
						textAnchor="middle"
						fontSize="9"
						fill="#938a7e"
					/>
				))}
			</svg>
		</Panel>
	);
};
