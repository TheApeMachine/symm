import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { StatusBadge } from "#/components/dashboard/status-badge";
import {
	headlineMetric,
	latestByMetric,
	percentOf,
	resolveStatus,
} from "#/components/terminal/measurement-view";

export const Meter = ({
	label,
	value,
	percent,
	color = "var(--info)",
}: {
	label: string;
	value: string;
	percent: number;
	color?: string;
}) => (
	<div className="flex items-center gap-2">
		<span className="w-[58px] text-[10px] text-(--f4)">{label}</span>
		<div className="h-[5px] flex-1 overflow-hidden rounded-[3px] bg-(--line)">
			<div
				className="h-full"
				style={{ width: `${percent}%`, backgroundColor: color }}
			/>
		</div>
		<span className="w-[18px] text-right font-mono text-[10px] text-(--f2)">
			{value}
		</span>
	</div>
);

export const Stat = ({
	value,
	label,
	accent = false,
}: {
	value: string;
	label: string;
	accent?: boolean;
}) => (
	<div>
		<div
			className="font-mono font-semibold text-2xl leading-none"
			style={{ color: accent ? "var(--acc)" : "var(--f1)" }}
		>
			{value}
		</div>
		<div className="mt-1 text-[9px] text-(--f4)">{label}</div>
	</div>
);

export type HealthSummary = {
	bars: Array<{ color: string; count: number; label: string; percent: number }>;
	avg: number;
	firing: number;
	label: string;
	measured: number;
	tone: string;
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
	const tone =
		degraded > 0
			? "var(--down)"
			: measured < total / 2
				? "var(--warn)"
				: "var(--up)";
	const bars = [
		{ label: "Healthy", count: measured, color: "var(--up)" },
		{ label: "Warming", count: warming, color: "var(--warn)" },
		{ label: "Degraded", count: degraded, color: "var(--down)" },
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
		tone,
		total,
	};
};

export const HealthPanel = ({ sources }: { sources?: string[] }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const health = terminalHealthSummary(readings, focusSymbol, sources);

	return (
		<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-[13px]">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">System health</span>
				<StatusBadge label={health.label} tone={health.tone} />
			</div>
			<div className="mt-3 flex gap-[18px]">
				<Stat value={`${health.measured}/${health.total}`} label="healthy" />
				<Stat value={`${health.avg}%`} label="avg strength" />
				<Stat value={String(health.firing)} label="firing" accent />
			</div>
			<div className="mt-[13px] flex flex-col gap-1.5">
				{health.bars.map((bar) => (
					<Meter
						key={bar.label}
						label={bar.label}
						value={String(bar.count)}
						percent={bar.percent}
						color={bar.color}
					/>
				))}
			</div>
		</div>
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
		<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-[13px]">
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
		</div>
	);
};
