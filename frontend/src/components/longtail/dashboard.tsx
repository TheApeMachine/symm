import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { bookAtom } from "#/collections/book";
import {
	alertVariant,
	buildInsights,
	type ExitMix,
	type LongtailSample,
	matchTone,
	pct,
	resistanceTone,
	returnTone,
	type Tone,
	tierStats,
	toneColor,
	toneVariant,
	topExitTriggers,
	useLongtailReport,
	verdict,
	winRateTone,
} from "#/collections/longtail";
import { Alert, Badge, Chip } from "#/components/ui";
import { Flex } from "#/components/ui/flex";
import { deskRowsForStats } from "#/lib/lifecycle";
import { formatExitTrigger, triggerTone } from "#/lib/trigger-labels";
import { cn } from "#/lib/utils";
import { type ChartSeries, MetricChart } from "./metric-chart";

/*
Longtail surface: selected-tier trajectory — win/edge, path-book resistance,
policy hits, exit mix, and stop pressure. All readouts follow the top-bar book.
*/

const rate = (value: number) => `${Math.round(value * 100)}%`;
const countLabel = (value: number) => `${Math.round(value)}`;
const resistLabel = (value: number) => value.toFixed(2);

const sparkPath = (
	points: number[],
	width = 100,
	height = 16,
	padding = 2,
): string => {
	if (points.length < 2) return "";

	let min = points[0];
	let max = points[0];
	for (let i = 1; i < points.length; i++) {
		if (points[i] < min) min = points[i];
		if (points[i] > max) max = points[i];
	}

	const range = max - min;
	let d = "";

	for (let i = 0; i < points.length; i++) {
		const x = (i / (points.length - 1)) * width;
		const norm = range > 0 ? (points[i] - min) / range : 0.5;
		const y = height - padding - norm * (height - padding * 2);
		d += `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)} `;
	}

	return d;
};

type Kpi = {
	id: string;
	label: string;
	value: (
		sample: LongtailSample,
		book: ReturnType<typeof bookAtom.get>,
	) => number | null;
	format: (value: number) => string;
	formatDelta: (delta: number) => string;
	tone: (value: number | null, sample?: LongtailSample) => Tone;
	higherIsBetter: boolean;
	colorDelta: boolean;
	epsilon: number;
};

const kpis: Kpi[] = [
	{
		id: "win",
		label: "win rate",
		value: (s, book) => tierStats(s, book).winRate,
		format: rate,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: winRateTone,
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.02,
	},
	{
		id: "edge",
		label: "edge",
		value: (s, book) => tierStats(s, book).edge,
		format: pct,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(2)}pp`,
		tone: returnTone,
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.002,
	},
	{
		id: "closed",
		label: "settled",
		value: (s, book) => tierStats(s, book).stats.closed,
		format: countLabel,
		formatDelta: (delta) => `${delta >= 0 ? "+" : ""}${Math.round(delta)}`,
		tone: () => "info",
		higherIsBetter: true,
		colorDelta: false,
		epsilon: 0.5,
	},
	{
		id: "promoted",
		label: "paper floor",
		value: (s) => s.paths.promoted,
		format: countLabel,
		formatDelta: (delta) => `${delta >= 0 ? "+" : ""}${Math.round(delta)}`,
		tone: (value) => (value === null ? "neutral" : value > 0 ? "good" : "info"),
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.5,
	},
	{
		id: "resistance",
		label: "mean resist",
		value: (s) => s.paths.meanResistance,
		format: resistLabel,
		formatDelta: (delta) => `${delta >= 0 ? "+" : ""}${delta.toFixed(2)}`,
		tone: (value, sample) =>
			resistanceTone(value, sample?.eligibleResistance ?? 0.2),
		higherIsBetter: false,
		colorDelta: true,
		epsilon: 0.02,
	},
	{
		id: "policy",
		label: "policy hit",
		value: (s) => s.policy.hitRate,
		format: rate,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: matchTone,
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.02,
	},
];

const KpiTile = ({
	kpi,
	samples,
	book,
}: {
	kpi: Kpi;
	samples: LongtailSample[];
	book: ReturnType<typeof bookAtom.get>;
}) => {
	const series = samples.map((s) => kpi.value(s, book));
	const lastSample = samples[samples.length - 1];
	const latest = lastSample ? kpi.value(lastSample, book) : null;
	const priorSample = samples[Math.max(0, samples.length - 16)];
	const prior = priorSample ? kpi.value(priorSample, book) : null;
	const delta =
		latest !== null && prior !== null && Number.isFinite(prior)
			? latest - prior
			: null;

	const tone = kpi.tone(latest, lastSample);
	const deltaTone: Tone =
		!kpi.colorDelta || delta === null || Math.abs(delta) < kpi.epsilon
			? "neutral"
			: delta > 0 === kpi.higherIsBetter
				? "good"
				: "bad";

	const sparkPoints = series
		.slice(-60)
		.filter(
			(v): v is number => v !== null && v !== undefined && Number.isFinite(v),
		);

	return (
		<div
			data-kpi={kpi.id}
			className="flex min-w-0 flex-col justify-between border-r border-(--line) px-2.5 py-1.5 last:border-r-0"
		>
			<div className="flex items-baseline justify-between gap-1">
				<span className="truncate text-[9px] uppercase tracking-[0.08em] text-(--f4)">
					{book} {kpi.label}
				</span>
				<span
					className="shrink-0 text-[9px] tabular-nums"
					style={{ color: toneColor(deltaTone) }}
				>
					{delta === null ? "—" : kpi.formatDelta(delta)}
				</span>
			</div>
			<span
				className="text-[15px] font-bold leading-tight tabular-nums"
				style={{ color: toneColor(tone) }}
			>
				{latest === null ? "—" : kpi.format(latest)}
			</span>
			<svg
				viewBox="0 0 100 16"
				preserveAspectRatio="none"
				className="h-3.5 w-full"
				aria-hidden
			>
				<title>Sparkline</title>
				<path
					d={sparkPath(sparkPoints)}
					fill="none"
					stroke={toneColor(tone)}
					strokeWidth={1.3}
					vectorEffect="non-scaling-stroke"
					opacity={0.85}
				/>
			</svg>
		</div>
	);
};

type ChartDef = {
	id: string;
	label: string;
	caption: string;
	series: (
		samples: LongtailSample[],
		book: ReturnType<typeof bookAtom.get>,
	) => ChartSeries[];
	formatY: (value: number) => string;
	yDomain?: [number, number];
	baseline?: number;
	tone: (
		sample: LongtailSample | undefined,
		book: ReturnType<typeof bookAtom.get>,
	) => Tone;
};

const charts: ChartDef[] = [
	{
		id: "edge",
		label: "Edge trajectory",
		caption: "selected-tier mean return and win rate over the sample window",
		series: (samples, book) => [
			{
				label: "edge",
				values: samples.map((s) => tierStats(s, book).edge),
				stroke: "var(--acc)",
			},
			{
				label: "win",
				values: samples.map((s) => tierStats(s, book).winRate),
				stroke: "var(--up)",
			},
		],
		formatY: (value) =>
			Math.abs(value) <= 1.5 ? pct(value) : countLabel(value),
		baseline: 0,
		tone: (s, book) => (s ? returnTone(tierStats(s, book).edge) : "neutral"),
	},
	{
		id: "paths",
		label: "Path book",
		caption: "developed leaves, paper-floor promotions, and mean resistance",
		series: (samples) => [
			{
				label: "developed",
				values: samples.map((s) => s.paths.developed),
				stroke: "var(--info)",
			},
			{
				label: "promoted",
				values: samples.map((s) => s.paths.promoted),
				stroke: "var(--up)",
			},
			{
				label: "resistance",
				values: samples.map((s) => s.paths.meanResistance),
				stroke: "var(--warn)",
			},
		],
		formatY: (value) => (value <= 1.5 ? resistLabel(value) : countLabel(value)),
		tone: (s) =>
			s
				? resistanceTone(s.paths.meanResistance, s.eligibleResistance)
				: "neutral",
	},
	{
		id: "churn",
		label: "Path churn",
		caption: "keys added vs removed between samples — thrash when both spike",
		series: (samples) => [
			{
				label: "added",
				values: samples.map((s) => s.paths.added),
				stroke: "var(--up)",
			},
			{
				label: "removed",
				values: samples.map((s) => s.paths.removed),
				stroke: "var(--down)",
			},
		],
		formatY: countLabel,
		tone: (s) => {
			if (!s || s.paths.added === null) return "neutral";
			const churn = (s.paths.added ?? 0) + (s.paths.removed ?? 0);
			return churn >= 3 ? "warn" : "info";
		},
	},
	{
		id: "policy",
		label: "Policy match",
		caption: "share of live symbol cuts that landed on a stored path",
		series: (samples) => [
			{
				label: "hit rate",
				values: samples.map((s) => s.policy.hitRate),
				stroke: "var(--acc)",
			},
		],
		formatY: rate,
		yDomain: [0, 1],
		tone: (s) => matchTone(s?.policy.hitRate ?? null),
	},
	{
		id: "exits",
		label: "Exit mix",
		caption: "profit vs stop vs policy exits on the selected tier",
		series: (samples, book) => {
			const mix = (s: LongtailSample): ExitMix => s.exits[book];
			return [
				{
					label: "profit",
					values: samples.map((s) => mix(s).profit),
					stroke: "var(--up)",
				},
				{
					label: "stop",
					values: samples.map((s) => mix(s).stop),
					stroke: "var(--down)",
				},
				{
					label: "policy",
					values: samples.map((s) => mix(s).policy),
					stroke: "var(--info)",
				},
			];
		},
		formatY: countLabel,
		tone: (s, book) => {
			if (!s) return "neutral";
			const mix = s.exits[book];
			if (mix.total === 0) return "neutral";
			return mix.profit > mix.stop
				? "good"
				: mix.stop > mix.profit
					? "warn"
					: "info";
		},
	},
	{
		id: "stops",
		label: "Stop proximity",
		caption:
			"open selected-tier book vs invalidation / trail / damage — 1.0 fires",
		series: (samples, book) => [
			{
				label: "stop loss",
				values: samples.map((s) => s.stops[book].invalidation),
				stroke: "var(--down)",
			},
			{
				label: "trail",
				values: samples.map((s) => s.stops[book].trailing),
				stroke: "var(--up)",
			},
			{
				label: "damage",
				values: samples.map((s) => s.stops[book].damage),
				stroke: "var(--warn)",
			},
		],
		formatY: rate,
		yDomain: [0, 1.25],
		baseline: 1,
		tone: (s, book) => {
			const stops = s?.stops[book];
			if (!stops || stops.maxProximity === null) return "neutral";
			return stops.maxProximity >= 1
				? "bad"
				: stops.maxProximity >= 0.7
					? "warn"
					: "good";
		},
	},
];

const CHART_ROW_COUNT = Math.ceil(charts.length / 2);

const ChartPanel = ({
	chart,
	samples,
	times,
	book,
	className,
}: {
	chart: ChartDef;
	samples: LongtailSample[];
	times: number[];
	book: ReturnType<typeof bookAtom.get>;
	className?: string;
}) => {
	const latestSample = samples[samples.length - 1];
	const series = chart.series(samples, book);
	const latestOf = (values: Array<number | null | undefined>) => {
		const tail = values[values.length - 1];
		return tail !== null && tail !== undefined && Number.isFinite(tail)
			? tail
			: null;
	};
	const heads = series.map((item) => {
		const value = latestOf(item.values);
		return value === null ? "—" : chart.formatY(value);
	});
	const headline = heads.every((head) => head === "—")
		? "—"
		: heads.join(" · ");
	const tone = chart.tone(latestSample, book);

	return (
		<div
			data-chart={chart.id}
			className={cn(
				"flex h-full min-h-0 min-w-0 flex-col border-b border-r border-(--line) bg-(--bg)",
				className,
			)}
		>
			<div className="flex shrink-0 items-baseline justify-between gap-2 border-b border-(--line) px-2.5 py-1">
				<div className="min-w-0">
					<div className="truncate text-[10px] font-bold text-(--f1)">
						{chart.label}
					</div>
					<div className="truncate text-[9px] text-(--f4)">{chart.caption}</div>
				</div>
				<span
					className="shrink-0 text-[11px] font-bold tabular-nums"
					style={{ color: toneColor(tone) }}
				>
					{headline}
				</span>
			</div>
			<MetricChart
				className="min-h-0 flex-1"
				times={times}
				series={series}
				formatY={chart.formatY}
				yDomain={chart.yDomain}
				baseline={chart.baseline}
			/>
		</div>
	);
};

const RailRow = ({
	label,
	value,
	meter,
	tone,
}: {
	label: string;
	value: string;
	meter?: number | null;
	tone: Tone;
}) => (
	<div className="px-2.5 py-1">
		<div className="flex items-baseline justify-between text-[10px]">
			<span className="text-(--f3)">{label}</span>
			<span
				className="font-bold tabular-nums"
				style={{ color: toneColor(tone) }}
			>
				{value}
			</span>
		</div>
		{meter !== undefined && meter !== null ? (
			<div className="mt-1 h-1 overflow-hidden rounded-xs bg-(--line)">
				<div
					className="h-full"
					style={{
						width: `${Math.max(0, Math.min(100, meter * 100))}%`,
						background: toneColor(tone),
					}}
				/>
			</div>
		) : null}
	</div>
);

const RailBlock = ({
	title,
	children,
}: {
	title: string;
	children: React.ReactNode;
}) => (
	<div className="shrink-0 border-b border-(--line)">
		<div className="border-b border-(--line) bg-(--surface) px-2.5 py-1 text-[9px] font-bold uppercase tracking-widest text-(--f3)">
			{title}
		</div>
		<div className="bg-(--bg) py-1">{children}</div>
	</div>
);

export const LongtailDashboard = () => {
	const { samples } = useLongtailReport();
	const book = useSelector(bookAtom, (state) => state);

	const times = useMemo(() => samples.map((s) => s.at), [samples]);
	const insights = useMemo(() => buildInsights(samples, book), [samples, book]);
	const v = useMemo(() => verdict(samples, book), [samples, book]);
	const exitTriggers = topExitTriggers(deskRowsForStats(), book);

	const latest = samples[samples.length - 1];
	const first = samples[0];
	const spanMinutes =
		latest && first ? Math.round((latest.at - first.at) / 60_000) : 0;
	const windowMinutes = Math.max(1, spanMinutes);
	const tier = latest ? tierStats(latest, book) : null;
	const paths = latest?.paths;
	const policy = latest?.policy;
	const exits = latest?.exits[book];
	const stops = latest?.stops[book];

	return (
		<Flex.Column className="h-full min-h-0 w-full overflow-hidden bg-(--bg) font-mono text-(--f2)">
			<Flex.Row
				align="center"
				justify="between"
				wrap="wrap"
				gap={2}
				data-book={book}
				className="shrink-0 border-b border-(--line) bg-(--surface) px-3 py-1.5"
			>
				<Flex.Row align="center" gap={2}>
					<span className="text-[11px] font-bold uppercase tracking-wide text-(--f1)">
						Longtail
					</span>
					<Badge
						variant={toneVariant(v.tone)}
						size="s"
						dot
						pulse={v.tone === "bad"}
						label={v.label}
					/>
				</Flex.Row>
				<Flex.Row gap={1} wrap="wrap">
					<Chip
						variant="quiet"
						size="s"
						label={book}
						value={
							tier
								? `${tier.stats.open} open · ${tier.stats.closed} settled`
								: "no data"
						}
					/>
					{book === "real" ? (
						<Chip
							variant="quiet"
							size="s"
							label="armed"
							value={
								tier?.armed === null || tier?.armed === undefined
									? "—"
									: tier.armed >= 0.5
										? "yes"
										: "halted"
							}
						/>
					) : null}
					<Chip
						variant="quiet"
						size="s"
						label="window"
						value={`${windowMinutes}m`}
					/>
					<Chip
						variant="quiet"
						size="s"
						label="samples"
						value={samples.length}
					/>
				</Flex.Row>
			</Flex.Row>

			<div className="grid shrink-0 grid-cols-6 border-b border-(--line) bg-(--surface)">
				{kpis.map((kpi) => (
					<KpiTile key={kpi.id} kpi={kpi} samples={samples} book={book} />
				))}
			</div>

			<Flex.Row className="min-h-0 flex-1 overflow-hidden">
				<div
					className="grid h-full min-h-0 min-w-0 flex-1 grid-cols-2"
					style={{
						gridTemplateRows: `repeat(${CHART_ROW_COUNT}, minmax(0, 1fr))`,
					}}
				>
					{charts.map((chart) => (
						<ChartPanel
							key={chart.id}
							chart={chart}
							samples={samples}
							times={times}
							book={book}
						/>
					))}
				</div>

				<Flex.Column className="min-h-0 w-72 shrink-0 border-l border-(--line) bg-(--bg)">
					<RailBlock title={`${book} book`}>
						<RailRow
							label="win rate"
							value={
								tier?.winRate === null || tier?.winRate === undefined
									? "—"
									: pct(tier.winRate)
							}
							meter={tier?.winRate ?? null}
							tone={winRateTone(tier?.winRate ?? null)}
						/>
						<RailRow
							label="edge"
							value={
								tier?.edge === null || tier?.edge === undefined
									? "—"
									: pct(tier.edge)
							}
							tone={returnTone(tier?.edge ?? null)}
						/>
						<RailRow
							label="settled"
							value={tier ? String(tier.stats.closed) : "—"}
							tone="info"
						/>
						<RailRow
							label="open"
							value={tier ? String(tier.stats.open) : "—"}
							tone="info"
						/>
						{book === "real" ? (
							<RailRow
								label="armed"
								value={
									!tier || tier.armed == null
										? "—"
										: tier.armed >= 0.5
											? "yes"
											: "halted"
								}
								tone={
									!tier || tier.armed == null
										? "neutral"
										: tier.armed >= 0.5
											? "good"
											: "warn"
								}
							/>
						) : null}
					</RailBlock>

					<RailBlock title="Paths">
						<RailRow
							label="developed"
							value={
								paths?.developed === null || paths?.developed === undefined
									? "—"
									: String(paths.developed)
							}
							tone="info"
						/>
						<RailRow
							label="paper floor"
							value={
								paths?.promoted === null || paths?.promoted === undefined
									? "—"
									: String(paths.promoted)
							}
							tone={(paths?.promoted ?? 0) > 0 ? "good" : "neutral"}
						/>
						<RailRow
							label="mean resistance"
							value={
								paths?.meanResistance === null ||
								paths?.meanResistance === undefined
									? "—"
									: paths.meanResistance.toFixed(2)
							}
							tone={resistanceTone(
								paths?.meanResistance ?? null,
								latest?.eligibleResistance ?? 0.2,
							)}
						/>
						<RailRow
							label="churn"
							value={
								paths?.added === null || paths?.added === undefined
									? "—"
									: `+${paths.added}/−${paths.removed ?? 0}`
							}
							tone={
								(paths?.added ?? 0) + (paths?.removed ?? 0) >= 3
									? "warn"
									: "info"
							}
						/>
					</RailBlock>

					<RailBlock title="Policy">
						<RailRow
							label="hit rate"
							value={
								policy?.hitRate === null || policy?.hitRate === undefined
									? "—"
									: pct(policy.hitRate)
							}
							meter={policy?.hitRate ?? null}
							tone={matchTone(policy?.hitRate ?? null)}
						/>
						<RailRow
							label="checks"
							value={policy ? String(policy.checks) : "—"}
							tone="info"
						/>
						<RailRow
							label="hits"
							value={policy ? String(policy.hits) : "—"}
							tone="info"
						/>
					</RailBlock>

					<RailBlock title="Exits">
						<RailRow
							label="profit"
							value={exits ? String(exits.profit) : "—"}
							tone={(exits?.profit ?? 0) > 0 ? "good" : "neutral"}
						/>
						<RailRow
							label="stop"
							value={exits ? String(exits.stop) : "—"}
							tone={(exits?.stop ?? 0) > 0 ? "warn" : "neutral"}
						/>
						<RailRow
							label="policy"
							value={exits ? String(exits.policy) : "—"}
							tone="info"
						/>
						<RailRow
							label="other"
							value={exits ? String(exits.other) : "—"}
							tone="neutral"
						/>
						{exitTriggers.length > 0 ? (
							<div className="mt-2 space-y-1 border-t border-(--line)/60 pt-2">
								<div className="px-2.5 text-[9px] font-bold uppercase tracking-wider text-(--f4)">
									top triggers
								</div>
								{exitTriggers.map(({ trigger, count }) => (
									<RailRow
										key={trigger}
										label={formatExitTrigger(trigger)}
										value={String(count)}
										tone={triggerTone(trigger)}
									/>
								))}
							</div>
						) : null}
					</RailBlock>

					<RailBlock title="Stops">
						<RailRow
							label="stop loss"
							value={
								stops?.invalidation === null ||
								stops?.invalidation === undefined
									? "—"
									: pct(stops.invalidation)
							}
							meter={stops?.invalidation ?? null}
							tone={
								(stops?.invalidation ?? 0) >= 1
									? "bad"
									: (stops?.invalidation ?? 0) >= 0.7
										? "warn"
										: (stops?.invalidation ?? 0) > 0
											? "info"
											: "neutral"
							}
						/>
						<RailRow
							label="trail"
							value={
								stops?.trailing === null || stops?.trailing === undefined
									? "—"
									: pct(stops.trailing)
							}
							meter={stops?.trailing ?? null}
							tone={
								(stops?.trailing ?? 0) >= 1
									? "warn"
									: (stops?.trailing ?? 0) > 0
										? "info"
										: "neutral"
							}
						/>
						<RailRow
							label="damage"
							value={
								stops?.damage === null || stops?.damage === undefined
									? "—"
									: pct(stops.damage)
							}
							meter={stops?.damage ?? null}
							tone={
								(stops?.damage ?? 0) >= 0.8
									? "warn"
									: (stops?.damage ?? 0) > 0
										? "info"
										: "neutral"
							}
						/>
						<RailRow
							label="lots armed"
							value={
								stops?.armed === null || stops?.armed === undefined
									? "—"
									: `${stops.armed}/${stops.open ?? "?"} at ≥70%`
							}
							tone={(stops?.armed ?? 0) > 0 ? "warn" : "neutral"}
						/>
						<RailRow
							label="nearest exit"
							value={
								stops?.maxProximity === null ||
								stops?.maxProximity === undefined
									? "—"
									: pct(stops.maxProximity)
							}
							tone={
								(stops?.maxProximity ?? 0) >= 1
									? "bad"
									: (stops?.maxProximity ?? 0) >= 0.7
										? "warn"
										: "neutral"
							}
						/>
					</RailBlock>

					<div className="min-h-0 flex-1 overflow-auto">
						<div className="border-b border-(--line) bg-(--surface) px-2.5 py-1 text-[9px] font-bold uppercase tracking-widest text-(--f3)">
							Plain-language readings
						</div>
						{insights.map((insight) => (
							<Alert
								key={insight.id}
								variant={alertVariant(insight.tone)}
								size="s"
								icon={false}
								className="border-x-0 border-t-0"
							>
								<span className="block font-bold text-(--f1)">
									{insight.title}
								</span>
								<span className="mt-1 block text-(--f2) opacity-90">
									{insight.body}
								</span>
							</Alert>
						))}
					</div>
				</Flex.Column>
			</Flex.Row>
		</Flex.Column>
	);
};
