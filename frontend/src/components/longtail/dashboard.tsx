import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { bookAtom } from "#/collections/book";
import {
	alertVariant,
	buildInsights,
	coverageTone,
	fixed,
	frictionPassTone,
	gatePassTone,
	type LongtailSample,
	matchTone,
	pathStableTone,
	pct,
	returnTone,
	signedPct,
	surpriseTone,
	type Tone,
	toneColor,
	toneVariant,
	useLongtailReport,
	verdict,
	warmTone,
	winRateTone,
} from "#/collections/longtail";
import { Alert, Badge, Chip } from "#/components/ui";
import { Flex } from "#/components/ui/flex";
import { type ChartSeries, MetricChart } from "./metric-chart";

/*
The training performance surface. It answers one question — is the learning
system doing well, and is it improving? — from three sources: the desk's
settled round trips (edge), the live grammar feeds (structure, gate, policy),
and the trend of each over the session window.

Everything fits one screen: a verdict header, an eight-tile KPI strip with
deltas and sparklines, six measured charts, and a right rail of health
readings plus plain-language alerts. Charts draw in their container's actual
pixel size, so nothing stretches; every colour is contextual, so a bad number
never paints green.
*/

const rate = (value: number) => `${Math.round(value * 100)}%`;

const latestFinite = (values: Array<number | null | undefined>) => {
	for (let i = values.length - 1; i >= 0; i--) {
		const v = values[i];
		if (v !== null && v !== undefined && Number.isFinite(v)) {
			return v;
		}
	}
	return null;
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
		epsilon: 0.005,
	},
	{
		id: "win",
		label: `win rate (${book})`,
		value: (s) => s[book]?.winRate ?? 0,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: winRateTone,
		higherIsBetter: true,
		epsilon: 0.005,
	},
	{
		id: "coverage",
		label: "action coverage",
		value: (s) => s.training?.actionCoverage ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: coverageTone,
		higherIsBetter: true,
		epsilon: 0.005,
	},
	{
		id: "stable",
		label: "path stable",
		value: (s) => s.training?.pathStableRate ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: pathStableTone,
		higherIsBetter: true,
		epsilon: 0.005,
	},
	{
		id: "gate",
		label: "gate pass",
		value: (s) => s.training?.gatePassRate ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: gatePassTone,
		higherIsBetter: true,
		epsilon: 0.005,
	},
	{
		id: "surprise",
		label: "surprise",
		value: (s) => s.training?.avgSurprise ?? null,
		format: (v) => v.toFixed(2),
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${Math.abs(delta).toFixed(2)}`,
		tone: surpriseTone,
		higherIsBetter: false,
		epsilon: 0.02,
	},
	{
		id: "match",
		label: "policy match",
		value: (s) => s.training?.policyMatchRate ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: matchTone,
		higherIsBetter: true,
		epsilon: 0.005,
	},
	{
		id: "friction",
		label: "friction pass",
		value: (s) => s.training?.frictionPassRate ?? null,
		format: (v) => `${(100 * v).toFixed(0)}%`,
		formatDelta: (delta) =>
			`${delta >= 0 ? "+" : ""}${(100 * delta).toFixed(1)}pp`,
		tone: frictionPassTone,
		higherIsBetter: true,
		epsilon: 0.005,
	},
];

const KpiTile = ({ kpi, samples }: { kpi: Kpi; samples: LongtailSample[] }) => {
	const series = samples.map(kpi.value);
	const latest = latestFinite(series);
	const prior =
		series.length > 16
			? series[series.length - 16]
			: series.length > 0
				? series[0]
				: null;
	const delta =
		latest !== null && prior !== null && Number.isFinite(prior)
			? latest - prior
			: null;

	const tone = kpi.tone(latest);
	const deltaTone: Tone =
		delta === null || Math.abs(delta) < kpi.epsilon
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
		| "gatePassRate"
		| "avgSurprise"
		| "reuseRate"
		| "createRate"
		| "policyMatchRate"
		| "pathStableRate"
		| "frictionPassRate",
): number | null => {
	const value = sample?.training?.[key];
	return typeof value === "number" ? value : null;
};

const chartsFor = (book: "paper" | "virtual"): ChartDef[] => [
	{
		id: "win",
		label: `${book} win rate`,
		caption: "closed lots with derived net returns",
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
		caption: "average net return on priced lots",
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
		id: "gate",
		label: "Gate pass rate",
		caption: "chords the surprise gate accepted",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			return [
				{
					values: samples.map((s) => trainingPoint(s, "gatePassRate")),
					stroke: toneColor(
						gatePassTone(trainingPoint(latest, "gatePassRate")),
					),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: rate,
		yDomain: [0, 1],
		tone: (s) => gatePassTone(trainingPoint(s, "gatePassRate")),
	},
	{
		id: "surprise",
		label: "Mean surprise",
		caption: "predictive coder error per chord · lower is better",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			return [
				{
					values: samples.map((s) => trainingPoint(s, "avgSurprise")),
					stroke: toneColor(surpriseTone(trainingPoint(latest, "avgSurprise"))),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: (v) => v.toFixed(2),
		tone: (s) => surpriseTone(trainingPoint(s, "avgSurprise")),
	},
	{
		id: "stable",
		label: "Path stability",
		caption: "canonical policy path vs sequitur path (no drift)",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			return [
				{
					values: samples.map((s) => trainingPoint(s, "pathStableRate")),
					stroke: toneColor(
						pathStableTone(trainingPoint(latest, "pathStableRate")),
					),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: rate,
		yDomain: [0, 1],
		tone: (s) => pathStableTone(trainingPoint(s, "pathStableRate")),
	},
	{
		id: "match",
		label: "Policy match rate",
		caption: "live paths ending on a stored policy",
		series: (samples) => {
			const latest = samples[samples.length - 1];
			return [
				{
					values: samples.map((s) => trainingPoint(s, "policyMatchRate")),
					stroke: toneColor(
						matchTone(trainingPoint(latest, "policyMatchRate")),
					),
					fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
				},
			];
		},
		formatY: rate,
		yDomain: [0, 1],
		tone: (s) => matchTone(trainingPoint(s, "policyMatchRate")),
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
	const values = series[0]?.values ?? [];
	const tail = values[values.length - 1];
	const latest =
		tail !== null && tail !== undefined && Number.isFinite(tail) ? tail : null;
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
					{latest === null ? "—" : chart.formatY(latest)}
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
	const windowMinutes = Math.max(1, Math.round((samples.length * 5) / 60));
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
								? `${bookStats.open} open · ${bookStats.closed} closed`
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
					<RailBlock title="Grammar">
						<RailRow
							label="rules"
							value={
								training?.rules === null || training?.rules === undefined
									? "—"
									: String(training.rules)
							}
							tone="info"
						/>
						<RailRow
							label="compression"
							value={
								training?.rawLength && training.grammarSymbols !== null
									? `${((training.grammarSymbols / training.rawLength) * 100).toFixed(0)}% of raw`
									: "—"
							}
							tone="info"
						/>
						<RailRow
							label="avg rule depth"
							value={
								training?.avgRuleDepth === null ||
								training?.avgRuleDepth === undefined
									? "—"
									: fixed(training.avgRuleDepth, 1)
							}
							tone="info"
						/>
						<RailRow
							label="max rule depth"
							value={
								training?.maxRuleDepth === null ||
								training?.maxRuleDepth === undefined
									? "—"
									: String(training.maxRuleDepth)
							}
							tone="info"
						/>
						<RailRow
							label="action coverage"
							value={
								training?.actionCoverage === null ||
								training?.actionCoverage === undefined
									? "—"
									: pct(training.actionCoverage)
							}
							meter={training?.actionCoverage ?? null}
							tone={coverageTone(training?.actionCoverage ?? null)}
						/>
					</RailBlock>

					<RailBlock title="Surprise gate">
						<RailRow
							label="coders warm"
							value={
								training?.warmRate === null || training?.warmRate === undefined
									? "—"
									: pct(training.warmRate)
							}
							meter={training?.warmRate ?? null}
							tone={warmTone(training?.warmRate ?? null)}
						/>
						<RailRow
							label="pass rate"
							value={
								training?.gatePassRate === null ||
								training?.gatePassRate === undefined
									? "—"
									: pct(training.gatePassRate)
							}
							meter={training?.gatePassRate ?? null}
							tone={gatePassTone(training?.gatePassRate ?? null)}
						/>
						<RailRow
							label="mean surprise"
							value={
								training?.avgSurprise === null ||
								training?.avgSurprise === undefined
									? "—"
									: fixed(training.avgSurprise)
							}
							tone={surpriseTone(training?.avgSurprise ?? null)}
						/>
						<RailRow
							label="mean z"
							value={
								training?.avgZ === null || training?.avgZ === undefined
									? "—"
									: fixed(training.avgZ)
							}
							tone="info"
						/>
					</RailBlock>

					<RailBlock title="Ledger">
						<RailRow
							label="explored enter"
							value={
								training?.exploredEnter === null ||
								training?.exploredEnter === undefined
									? "—"
									: String(training.exploredEnter)
							}
							tone="info"
						/>
						<RailRow
							label="confirmed enter"
							value={
								training?.confirmedEnter === null ||
								training?.confirmedEnter === undefined
									? "—"
									: String(training.confirmedEnter)
							}
							tone="info"
						/>
						<RailRow
							label="venue open"
							value={
								training?.venueOpen === null ||
								training?.venueOpen === undefined
									? "—"
									: String(training.venueOpen)
							}
							tone="info"
						/>
						<RailRow
							label="virtual excursions"
							value={
								training?.virtualExcursion === null ||
								training?.virtualExcursion === undefined
									? "—"
									: String(training.virtualExcursion)
							}
							tone="info"
						/>
						<RailRow
							label="grid maturity"
							value={
								training?.meanMaturity === null ||
								training?.meanMaturity === undefined
									? "—"
									: pct(training.meanMaturity)
							}
							meter={training?.meanMaturity ?? null}
							tone={warmTone(training?.meanMaturity ?? null)}
						/>
					</RailBlock>

					<RailBlock title="Policy">
						<RailRow
							label="stored paths"
							value={
								training?.policyStoredRate === null ||
								training?.policyStoredRate === undefined
									? "—"
									: pct(training.policyStoredRate)
							}
							meter={training?.policyStoredRate ?? null}
							tone="info"
						/>
						<RailRow
							label="match rate"
							value={
								training?.policyMatchRate === null ||
								training?.policyMatchRate === undefined
									? "—"
									: pct(training.policyMatchRate)
							}
							meter={training?.policyMatchRate ?? null}
							tone={matchTone(training?.policyMatchRate ?? null)}
						/>
						<RailRow
							label="avg match depth"
							value={
								training?.avgMatchDepth === null ||
								training?.avgMatchDepth === undefined
									? "—"
									: fixed(training.avgMatchDepth, 1)
							}
							tone="info"
						/>
						<RailRow
							label="desk acted"
							value={
								training?.outcomeRate === null ||
								training?.outcomeRate === undefined
									? "—"
									: pct(training.outcomeRate)
							}
							meter={training?.outcomeRate ?? null}
							tone="info"
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
