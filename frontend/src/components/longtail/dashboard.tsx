import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import { positionRows } from "#/components/learning/open-positions";
import {
	buildInsights,
	ensureLongtailRecorder,
	type LongtailInsight,
	type LongtailSample,
	useLongtailReport,
} from "#/collections/longtail";
import {
	closedOnBook,
	openOnBook,
	positionReadout,
} from "#/components/book-readout";
import { Alert, Chip, formatValue, SeriesChart } from "#/components/ui";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Scanlines } from "#/components/ui/scanlines";
import { Section } from "#/components/ui/section";
import { cn } from "#/lib/utils";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo } from "react";

const insightVariant = (
	trend: LongtailInsight["trend"],
): "success" | "warning" | "info" => {
	if (trend === "up") return "success";
	if (trend === "down") return "warning";
	return "info";
};

const trendLabel = (trend: LongtailInsight["trend"]) => {
	switch (trend) {
		case "up":
			return "improving";
		case "down":
			return "slipping";
		default:
			return "steady";
	}
};

const pct = (value: number) => formatValue(value, "pct");
const rate = (value: number) => `${Math.round(value * 100)}%`;
const count = (value: number) => String(Math.round(value));

type MetricPanel = {
	id: string;
	label: string;
	caption: string;
	values: (samples: LongtailSample[]) => Array<number | null | undefined>;
	formatY: (value: number) => string;
	yDomain?: [number, number];
	baseline?: number;
	stroke: string;
	fill: string;
};

const metricsFor = (book: Book): MetricPanel[] => [
	{
		id: "win",
		label: `${book} win rate`,
		caption: `${book} closed lots with derived net returns`,
		values: (samples) => samples.map((sample) => sample[book].winRate),
		formatY: rate,
		yDomain: [0, 1],
		stroke: "var(--info)",
		fill: "color-mix(in srgb, var(--info) 18%, transparent)",
	},
	{
		id: "return",
		label: `${book} mean return`,
		caption: `Average net return on priced ${book} lots`,
		values: (samples) => samples.map((sample) => sample[book].meanReturn),
		formatY: pct,
		baseline: 0,
		stroke: "var(--up)",
		fill: "color-mix(in srgb, var(--up) 16%, transparent)",
	},
	{
		id: "gate",
		label: "Gate pass rate",
		caption: "Grammar chords accepted",
		values: (samples) => samples.map((s) => s.gatePassRate),
		formatY: rate,
		yDomain: [0, 1],
		stroke: "var(--acc)",
		fill: "color-mix(in srgb, var(--acc) 20%, transparent)",
	},
	{
		id: "policy",
		label: "Policy hit rate",
		caption: "Paths with stored action",
		values: (samples) => samples.map((s) => s.policyStoredRate),
		formatY: rate,
		yDomain: [0, 1],
		stroke: "var(--brand)",
		fill: "color-mix(in srgb, var(--brand) 18%, transparent)",
	},
	{
		id: "blocked",
		label: "Gate blocked rate",
		caption: "Chords stopped by surprise gate",
		values: (samples) => samples.map((s) => s.blockedRate),
		formatY: rate,
		yDomain: [0, 1],
		stroke: "var(--down)",
		fill: "color-mix(in srgb, var(--down) 18%, transparent)",
	},
	{
		id: "open",
		label: `${book} open lots`,
		caption: `Positions on the selected ${book} book`,
		values: (samples) => samples.map((sample) => sample[book].open),
		formatY: count,
		stroke: "var(--info)",
		fill: "color-mix(in srgb, var(--info) 14%, transparent)",
	},
];

const latestFinite = (values: Array<number | null | undefined>) => {
	for (let i = values.length - 1; i >= 0; i--) {
		const v = values[i];
		if (v !== null && v !== undefined && Number.isFinite(v)) {
			return v;
		}
	}
	return null;
};

export const LongtailDashboard = () => {
	useEffect(() => {
		ensureLongtailRecorder();
	}, []);

	const { samples } = useLongtailReport();
	const book = useSelector(bookAtom, (state) => state);
	const state = useSelector(signals.positions, (state) => state);
	const desk = useMemo(() => positionRows(state), [state]);
	const times = useMemo(() => samples.map((s) => s.at), [samples]);
	const metrics = metricsFor(book);
	const selectedInsights = buildInsights(samples, book);
	const selectedOpen = openOnBook(desk, book);
	const selectedClosed = closedOnBook(desk, book)
		.sort((left, right) =>
			(positionReadout(right).closedAt ?? "").localeCompare(
				positionReadout(left).closedAt ?? "",
			),
		)
		.slice(0, 16);

	return (
		<Flex.Column className="h-full min-h-0 w-full bg-(--bg) text-(--f2) font-mono">
			<Flex.Row
				className="shrink-0 border-(--line) border-b bg-(--surface) px-3 py-1.5"
				data-book={book}
				align="center"
				justify="between"
				wrap="wrap"
				gap={2}
			>
				<span className="text-[11px] font-bold uppercase tracking-wide text-(--f1)">
					Learning long-tail
				</span>
				<Flex.Row gap={1} wrap="wrap">
					<Chip
						variant="quiet"
						size="s"
						label={book}
						value={`${selectedOpen.length} open`}
					/>
					<Chip
						variant="quiet"
						size="s"
						label="samples"
						value={samples.length}
					/>
				</Flex.Row>
			</Flex.Row>

			<Flex.Row className="min-h-0 flex-1">
				<Section
					fit="pane"
					surface="none"
					className="flex min-h-0 min-w-0 flex-4 flex-col border-(--line) border-r"
				>
					<Section.Header
						title={`${book} session metrics`}
						size="s"
						meta="6 panels · local time"
					/>
					<Section.Body
						scroll={false}
						className="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-(--sunken) p-0"
					>
						<Scanlines variant="plate" />
						<div className="relative z-1 grid h-full min-h-0 flex-1 grid-cols-2 grid-rows-3">
							{metrics.map((metric) => {
								const series = metric.values(samples);
								const latest = latestFinite(series);
								const openDomain =
									metric.id === "open"
										? ([
												0,
												Math.max(
													1,
													...series.filter(
														(v): v is number =>
															v !== null &&
															v !== undefined &&
															Number.isFinite(v),
													),
												),
											] as [number, number])
										: metric.yDomain;

								return (
									<div
										key={metric.id}
										className="flex min-h-0 flex-col border-(--line) border-b border-r bg-(--bg)"
									>
										<Flex.Row
											justify="between"
											align="baseline"
											className="shrink-0 border-(--line) border-b px-2.5 py-1"
										>
											<div className="min-w-0">
												<div className="text-[10px] font-bold text-(--f1)">
													{metric.label}
												</div>
												<div className="text-[9px] text-(--f4)">
													{metric.caption}
												</div>
											</div>
											<span
												className="shrink-0 text-[11px] font-bold tabular-nums"
												style={{ color: metric.stroke }}
											>
												{latest === null ? "—" : metric.formatY(latest)}
											</span>
										</Flex.Row>
										<div className="relative min-h-0 flex-1">
											<SeriesChart
												className="absolute inset-0"
												times={times}
												values={series}
												formatY={metric.formatY}
												yDomain={openDomain}
												baseline={metric.baseline}
												stroke={metric.stroke}
												fill={metric.fill}
											/>
										</div>
									</div>
								);
							})}
						</div>
					</Section.Body>
				</Section>

				<Flex.Column className="min-h-0 min-w-0 flex-1">
					<Section fit="pane" surface="surface" className="min-h-0 flex-1">
						<Section.Header title="Plain-language alerts" size="s" sticky />
						<Section.Body scroll className="p-0">
							{selectedInsights.map((insight) => (
								<Alert
									key={insight.id}
									variant={insightVariant(insight.trend)}
									size="s"
									icon={false}
									className="border-x-0 border-t-0"
								>
									<span className="block font-bold text-(--f1)">
										{insight.title}
									</span>
									<span className="mt-0.5 block uppercase text-[9px] opacity-70">
										{trendLabel(insight.trend)}
									</span>
									<span className="mt-1 block text-(--f2) opacity-90">
										{insight.body}
									</span>
								</Alert>
							))}
						</Section.Body>
					</Section>

					<Section
						fit="pane"
						surface="surface"
						className="min-h-0 max-h-[42%] border-(--line) border-t"
					>
						<Section.Header title="Latest closed" size="s" />
						<Section.Body scroll className="p-0">
							{selectedClosed.length === 0 ? (
								<p className="px-2.5 py-2 text-[10px] text-(--f4)">
									No closed {book} lots yet.
								</p>
							) : (
								<List>
									{selectedClosed.map((row) => {
										const ret = positionReadout(row).returnPct;
										const tone =
											ret === null || ret === undefined
												? "muted"
												: ret >= 0
													? "up"
													: "down";
										return (
											<List.Item
												key={String(row.id)}
												interactive={false}
												justify="between"
												className="rounded-none border-(--line) border-b px-2.5 py-1.5 text-[10px]"
											>
												<span className="truncate text-(--f1)">
													{String(row.symbol ?? "")}
													<span className="text-(--f4)"> {book}</span>
												</span>
												<span
													className={cn(
														tone === "up"
															? "text-(--up)"
															: tone === "down"
																? "text-(--down)"
																: "text-(--f3)",
													)}
												>
													{ret === null || ret === undefined
														? "—"
														: formatValue(ret, "pct")}
												</span>
											</List.Item>
										);
									})}
								</List>
							)}
						</Section.Body>
					</Section>
				</Flex.Column>
			</Flex.Row>
		</Flex.Column>
	);
};
