import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import {
	headlineMetric,
	latestByMetric,
	percentOf,
	resolveStatus,
} from "#/components/terminal/measurement-view";
import { Badge } from "@/components/ui/badge";
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
		const history =
			focusSymbol === "stream"
				? Object.values(readings.measurements).flatMap(
						(sourceMap) => sourceMap[source]?.values() ?? [],
					)
				: (readings.measurements[focusSymbol]?.[source]?.values() ?? []);
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

		if (history.length > 0) {
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

export const HealthPanel = ({ sources }: { sources?: string[] }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const health = terminalHealthSummary(readings, focusSymbol, sources);

	return (
		<Panel size="lg">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">System health</span>
				<Badge label={health.label} variant={health.variant} />
			</div>
			<div className="mt-3 flex gap-[18px]">
				<Stat value={`${health.measured}/${health.total}`} label="healthy" />
				<Stat value={`${health.avg}%`} label="avg strength" />
				<Stat value={String(health.firing)} label="firing" variant="warning" />
			</div>
			<div className="mt-[13px] flex flex-col gap-1.5">
				{health.bars.map((bar) => (
					<Meter
						key={bar.label}
						layout="inline"
						label={bar.label}
						value={String(bar.count)}
						percent={bar.percent}
						variant={bar.variant}
					/>
				))}
			</div>
		</Panel>
	);
};

export type SignalAxis = { label: string; value: number };

/*
topSignalAxes ranks sources by their market-wide headline strength — the mean
of each source's latest headline metric across every symbol currently
reporting it. Sources without a headline metric (e.g. toxicity, which reports
raw liquidity stats rather than a composite score) are not rankable this way
and are excluded rather than faked into a score.
*/
export const topSignalAxes = (
	readings: typeof measurementsStore.state,
	sources: string[],
	slots = 5,
): SignalAxis[] => {
	const ranked = sources.flatMap((source) => {
		const headline = headlineMetric(source);

		if (headline === null) {
			return [];
		}

		const values = Object.values(readings.measurements).flatMap((sourceMap) => {
			const latest = latestByMetric(
				sourceMap[source]?.values() ?? [],
				headline,
			);

			return latest === undefined ? [] : [percentOf(latest) / 100];
		});

		if (values.length === 0) {
			return [];
		}

		return [
			{
				label: source,
				value: values.reduce((sum, value) => sum + value, 0) / values.length,
			},
		];
	});

	return ranked.sort((left, right) => right.value - left.value).slice(0, slots);
};

export const RadarPanel = ({ sources }: { sources?: string[] }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const candidates = sources ?? [
		...new Set(
			Object.values(readings.measurements).flatMap((sourceMap) =>
				Object.keys(sourceMap),
			),
		),
	];
	const axes = topSignalAxes(readings, candidates);
	const units = [
		[0, -1],
		[0.951, -0.309],
		[0.588, 0.809],
		[-0.588, 0.809],
		[-0.951, -0.309],
	];
	const points = units
		.map(
			([x, y], index) =>
				`${110 + x * 84 * (axes[index]?.value ?? 0)},${
					105 + y * 84 * (axes[index]?.value ?? 0)
				}`,
		)
		.join(" ");

	return (
		<Panel size="lg">
			<div className="mb-2 font-semibold text-(--f1) text-xs">
				Strongest signals
			</div>
			<div className="mb-2 font-mono text-[9.5px] text-(--f4)">
				cross-section mean strength · market
			</div>
			<svg viewBox="0 0 220 210" className="block w-full">
				<title>Strongest signals</title>
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
				{units.map(([x, y]) => (
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
					points={points}
					fill="rgba(232,163,61,0.22)"
					stroke="#e8a33d"
					strokeWidth="1.6"
				/>
				{units.map(([x, y], index) => (
					<text
						key={`${x}:${y}`}
						x={110 + x * 98}
						y={105 + y * 98}
						textAnchor="middle"
						fontSize="9"
						fill="#938a7e"
					>
						{axes[index]?.label ?? "—"}
					</text>
				))}
			</svg>
		</Panel>
	);
};
