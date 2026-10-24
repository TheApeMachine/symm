import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { bookAtom } from "#/collections/book";
import {
	alertVariant,
	buildInsights,
	type LongtailSample,
	pct,
	returnTone,
	signedPct,
	type Tone,
	toneColor,
	toneVariant,
	useLongtailReport,
	verdict,
	winRateTone,
} from "#/collections/longtail";
import { Alert, Badge, Chip } from "#/components/ui";
import { Flex } from "#/components/ui/flex";
import { type ChartSeries, MetricChart } from "./metric-chart";

/*
The training performance surface. It answers one question — is the book
making money, and is that changing? — from settled round trips and the path
book: developed paths, the paper floor, prunes, and why lots closed.

Everything fits one screen: a verdict header, an eight-tile KPI strip with
deltas and sparklines, six measured charts, and a right rail of health
readings plus plain-language alerts. Charts draw in their container's actual
pixel size, so nothing stretches; every colour is contextual, so a bad number
never paints green.
*/

const rate = (value: number) => `${Math.round(value * 100)}%`;
const countLabel = (value: number) => `${Math.round(value)}`;

const formatHold = (ns: number) => {
	const seconds = Math.abs(ns) / 1e9;
	if (seconds < 90) return `${seconds.toFixed(0)}s`;
	const minutes = seconds / 60;
	if (minutes < 90) return `${minutes.toFixed(0)}m`;
	return `${(minutes / 60).toFixed(1)}h`;
};

const holdTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 180e9
			? "good"
			: value >= 30e9
				? "warn"
				: "bad";

const levelTone = (_value: number | null): Tone => "neutral";

const branchShare = (sample: LongtailSample): number | null => {
	const training = sample.training;
	if (
		training?.exitProfit === null ||
		training?.exitProfit === undefined ||
		training.exitStop === null ||
		training.exitStop === undefined ||
		training.exitBranch === null ||
		training.exitBranch === undefined
	) {
		return null;
	}
	const total = training.exitProfit + training.exitStop + training.exitBranch;
	if (total === 0) return null;
	return training.exitBranch / total;
};

/* A flat sparkline sits mid-height instead of pinning to the floor. */
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
	value: (sample: LongtailSample) => number | null;
	format: (value: number) => string;
	formatDelta: (delta: number) => string;
	tone: (value: number | null) => Tone;
	higherIsBetter: boolean;
	/** When false, movement stays grey. Counts are context, not a score. */
	colorDelta: boolean;
	epsilon: number;
};

const kpisFor = (book: "paper" | "virtual"): Kpi[] => [
	{
		id: "edge",
		label: `edge (${book})`,
		value: (s) => s[book]?.meanReturn ?? null,
		format: signedPct,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: returnTone,
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.005,
	},
	{
		id: "win",
		label: `win rate (${book})`,
		value: (s) => s[book]?.winRate ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: winRateTone,
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.005,
	},
	{
		id: "hold",
		label: "avg hold",
		value: (s) => s[book]?.avgHoldNs ?? null,
		format: formatHold,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : "−"}${formatHold(Math.abs(delta))}`,
		tone: holdTone,
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 5e9,
	},
	{
		id: "letters",
		label: "new letters",
		value: (s) => s.training?.tokenEmitRate ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: levelTone,
		higherIsBetter: true,
		colorDelta: false,
		epsilon: 0.02,
	},
	{
		id: "developed",
		label: "paths",
		value: (s) => s.training?.developedPaths ?? null,
		format: countLabel,
		formatDelta: (delta) => `${delta >= 0 ? "+" : ""}${Math.round(delta)}`,
		tone: levelTone,
		higherIsBetter: true,
		colorDelta: false,
		epsilon: 0.5,
	},
	{
		id: "promoted",
		label: "paper paths",
		value: (s) => s.training?.promotedPaths ?? null,
		format: countLabel,
		formatDelta: (delta) => `${delta >= 0 ? "+" : ""}${Math.round(delta)}`,
		tone: (value) => (value !== null && value > 0 ? "good" : "neutral"),
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.5,
	},
	{
		id: "pruned",
		label: "pruned",
		value: (s) => s.training?.prunedPaths ?? null,
		format: countLabel,
		formatDelta: (delta) => `${delta >= 0 ? "+" : ""}${Math.round(delta)}`,
		tone: levelTone,
		higherIsBetter: false,
		colorDelta: false,
		epsilon: 0.5,
	},
	{
		id: "branch",
		label: "branch exits",
		value: branchShare,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: (value) => (value !== null && value > 0 ? "good" : "neutral"),
		higherIsBetter: true,
		colorDelta: true,
		epsilon: 0.02,
	},
];

const KpiTile = ({ kpi, samples }: { kpi: Kpi; samples: LongtailSample[] }) => {
	const series = samples.map(kpi.value);
	const lastSample = samples[samples.length - 1];
	const latest = lastSample ? kpi.value(lastSample) : null;
	const priorSample = samples[Math.max(0, samples.length - 16)];
	const prior = priorSample ? kpi.value(priorSample) : null;
	const delta =
		latest !== null && prior !== null && Number.isFinite(prior)
			? latest - prior
			: null;

	const tone = kpi.tone(latest);
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
					{kpi.label}
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
	series: (samples: LongtailSample[]) => ChartSeries[];
	formatY: (value: number) => string;
	yDomain?: [number, number];
	baseline?: number;
	tone: (sample: LongtailSample | undefined) => Tone;
};

const bookPoint = (
	sample: LongtailSample | undefined,
	book: "paper" | "virtual",
	key: "winRate" | "meanReturn",
): number | null => sample?.[book]?.[key] ?? null;

const trainingPoint = (
	sample: LongtailSample | undefined,
	key:
		| "tokenEmitRate"
		| "blockedRate"
		| "reuseRate"
		| "createRate"
		| "policyStoredRate"
		| "outcomeRate",
): number | null => {
	const value = sample?.training?.[key];
	return typeof value === "number" ? value : null;
};

const chartsFor = (book: "paper" | "virtual"): ChartDef[] => [
	{
		id: "win",
		label: `${book} win rate`,
		caption: "settled lots whose book return is positive",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			return [
				{
					values: samples.map((s) => bookPoint(s, book, "winRate")),
					stroke: toneColor(winRateTone(bookPoint(latest, book, "winRate"))),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: rate,
		yDomain: [0, 1],
		baseline: 0.5,
		tone: (s) => winRateTone(bookPoint(s, book, "winRate")),
	},
	{
		id: "return",
		label: `${book} mean return`,
		caption: "mean desk pnl on settled lots (fraction)",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			return [
				{
					values: samples.map((s) => bookPoint(s, book, "meanReturn")),
					stroke: toneColor(returnTone(bookPoint(latest, book, "meanReturn"))),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: (v) => `${(100 * v).toFixed(1)}%`,
		baseline: 0,
		tone: (s) => returnTone(bookPoint(s, book, "meanReturn")),
	},
	{
		id: "hold",
		label: "Average hold",
		caption: "mean time a settled lot stayed open",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			const hold = latest?.[book]?.avgHoldNs ?? null;
			return [
				{
					values: samples.map((s) => {
						const ns = s[book]?.avgHoldNs;
						return ns === null || ns === undefined ? null : ns / 1e9;
					}),
					stroke: toneColor(holdTone(hold)),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: (v) => formatHold(v * 1e9),
		tone: (s) => holdTone(s?.[book]?.avgHoldNs ?? null),
	},
	{
		id: "letters",
		label: "New letters",
		caption: "symbol steps whose regime.posture letter changed",
		series: (samples) => [
			{
				values: samples.map((s) => trainingPoint(s, "tokenEmitRate")),
				stroke: "var(--info)",
				fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
			},
		],
		formatY: rate,
		yDomain: [0, 1] as [number, number],
		tone: () => "info" as Tone,
	},
	{
		id: "paths",
		label: "Path book",
		caption: "developed paths, and how many have reached the paper floor",
		series: (samples) => [
			{
				label: "developed",
				values: samples.map((s) => s.training?.developedPaths ?? null),
				stroke: "var(--info)",
			},
			{
				label: "paper",
				values: samples.map((s) => s.training?.promotedPaths ?? null),
				stroke: "var(--up)",
			},
		],
		formatY: countLabel,
		tone: () => "info" as Tone,
	},
	{
		id: "exits",
		label: "Exit reasons",
		caption: "profit hurdle, friction stop, and learned exit branch",
		series: (samples) => [
			{
				label: "profit",
				values: samples.map((s) => s.training?.exitProfit ?? null),
				stroke: "var(--up)",
			},
			{
				label: "stop",
				values: samples.map((s) => s.training?.exitStop ?? null),
				stroke: "var(--down)",
			},
			{
				label: "branch",
				values: samples.map((s) => s.training?.exitBranch ?? null),
				stroke: "var(--info)",
			},
		],
		formatY: countLabel,
		tone: (s) => {
			const profit = s?.training?.exitProfit ?? 0;
			const stop = s?.training?.exitStop ?? 0;
			if (profit + stop + (s?.training?.exitBranch ?? 0) === 0) return "neutral";
			return profit > stop ? "good" : "warn";
		},
	},
];

const ChartPanel = ({
	chart,
	samples,
	times,
}: {
	chart: ChartDef;
	samples: LongtailSample[];
	times: number[];
}) => {
	const latestSample = samples[samples.length - 1];
	const series = chart.series(samples);
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
	const headline = heads.every((head) => head === "—") ? "—" : heads.join(" · ");
	const tone = chart.tone(latestSample);

	return (
		<div
			data-chart={chart.id}
			className="flex min-h-0 min-w-0 flex-col border-b border-r border-(--line) bg-(--bg)"
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
				className="flex-1"
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
	const kpis = useMemo(() => kpisFor(book), [book]);
	const charts = useMemo(() => chartsFor(book), [book]);
	const insights = useMemo(() => buildInsights(samples, book), [samples, book]);
	const v = useMemo(() => verdict(samples, book), [samples, book]);

	const latest = samples[samples.length - 1];
	const first = samples[0];
	const spanMinutes =
		latest && first ? Math.round((latest.at - first.at) / 60_000) : 0;
	const windowMinutes = Math.max(1, spanMinutes);
	const training = latest?.training;
	const bookStats = latest ? latest[book] : undefined;

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
						Training performance
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
							bookStats
								? `${bookStats.open} open · ${bookStats.closed} settled`
								: "no data"
						}
					/>
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

			<div className="grid shrink-0 grid-cols-8 border-b border-(--line) bg-(--surface)">
				{kpis.map((kpi) => (
					<KpiTile key={kpi.id} kpi={kpi} samples={samples} />
				))}
			</div>

			<Flex.Row className="min-h-0 flex-1">
				<div className="grid min-h-0 min-w-0 flex-1 grid-cols-2 grid-rows-3">
					{charts.map((chart) => (
						<ChartPanel
							key={chart.id}
							chart={chart}
							samples={samples}
							times={times}
						/>
					))}
				</div>

				<Flex.Column className="min-h-0 w-72 shrink-0 border-l border-(--line) bg-(--bg)">
					<RailBlock title="Paths">
						<RailRow
							label="developed"
							value={
								training?.developedPaths === null ||
								training?.developedPaths === undefined
									? "—"
									: String(training.developedPaths)
							}
							tone="info"
						/>
						<RailRow
							label="paper floor"
							value={
								training?.promotedPaths === null ||
								training?.promotedPaths === undefined
									? "—"
									: String(training.promotedPaths)
							}
							tone={
								(training?.promotedPaths ?? 0) > 0 ? "good" : "neutral"
							}
						/>
						<RailRow
							label="pruned"
							value={
								training?.prunedPaths === null ||
								training?.prunedPaths === undefined
									? "—"
									: String(training.prunedPaths)
							}
							tone="info"
						/>
						<RailRow
							label="mean threshold"
							value={
								training?.meanThreshold === null ||
								training?.meanThreshold === undefined
									? "—"
									: training.meanThreshold.toFixed(2)
							}
							tone="info"
						/>
					</RailBlock>

					<RailBlock title="Letters">
						<RailRow
							label="new letter"
							value={
								training?.tokenEmitRate === null ||
								training?.tokenEmitRate === undefined
									? "—"
									: pct(training.tokenEmitRate)
							}
							meter={training?.tokenEmitRate ?? null}
							tone="info"
						/>
						<RailRow
							label="same letter"
							value={
								training?.blockedDuplicateRate === null ||
								training?.blockedDuplicateRate === undefined
									? "—"
									: pct(training.blockedDuplicateRate)
							}
							tone="info"
						/>
						<RailRow
							label="risk veto"
							value={
								training?.vetoRate === null || training?.vetoRate === undefined
									? "—"
									: pct(training.vetoRate)
							}
							tone="info"
						/>
						<RailRow
							label="dark role"
							value={
								training?.emptyRate === null || training?.emptyRate === undefined
									? "—"
									: pct(training.emptyRate)
							}
							tone="info"
						/>
					</RailBlock>

					<RailBlock title="Desk">
						<RailRow
							label="virtual open"
							value={
								latest?.virtual.open === undefined
									? "—"
									: String(latest.virtual.open)
							}
							tone="info"
						/>
						<RailRow
							label="paper open"
							value={
								latest?.paper.open === undefined
									? "—"
									: String(latest.paper.open)
							}
							tone="info"
						/>
						<RailRow
							label="enters last tick"
							value={
								training?.exploredEnter === null ||
								training?.exploredEnter === undefined
									? "—"
									: String(training.exploredEnter)
							}
							tone="info"
						/>
					</RailBlock>

					<RailBlock title="Exits">
						<RailRow
							label="profit hurdle"
							value={
								training?.exitProfit === null ||
								training?.exitProfit === undefined
									? "—"
									: String(training.exitProfit)
							}
							tone={(training?.exitProfit ?? 0) > 0 ? "good" : "neutral"}
						/>
						<RailRow
							label="friction stop"
							value={
								training?.exitStop === null || training?.exitStop === undefined
									? "—"
									: String(training.exitStop)
							}
							tone={(training?.exitStop ?? 0) > 0 ? "warn" : "neutral"}
						/>
						<RailRow
							label="exit branch"
							value={
								training?.exitBranch === null ||
								training?.exitBranch === undefined
									? "—"
									: String(training.exitBranch)
							}
							tone="info"
						/>
						<RailRow
							label="settled hold"
							value={
								bookStats?.avgHoldNs === null ||
								bookStats?.avgHoldNs === undefined
									? "—"
									: formatHold(bookStats.avgHoldNs)
							}
							tone={holdTone(bookStats?.avgHoldNs ?? null)}
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
