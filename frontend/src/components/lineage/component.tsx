import { useEffect, useMemo, useRef, useState } from "react";
import { Chip } from "#/components/ui/chip";
import { Flex } from "#/components/ui/flex";
import { Icon } from "#/components/ui/icon";
import { Panel } from "#/components/ui/panel";
import { Toolbar } from "#/components/ui/toolbar";
import { Typography } from "#/components/ui/typography";
import {
	type ConsumerRow,
	ensureLineageLoaded,
	LINEAGE_STATUS_COLOR as STATUS_COLOR,
	lineageStatusOf,
	type ProducerRow,
	readLineage,
	subscribeLineage,
} from "#/components/terminal/lineage-report";
import { cn } from "#/lib/utils";

type StatusFilter = "all" | "unreferenced" | "referenced" | "kernelOnly";

const statusOf = (p: ProducerRow): "unreferenced" | "referenced" | "kernelOnly" => lineageStatusOf(p) ?? "referenced";

/*
Point is one laid-out node: a kernel, a metric, or a consumer, positioned in
one of three columns. Metric y-positions are assigned within their own
kernel's vertical band, and kernel bands are sized proportionally to their
metric count — so a 63-metric kernel (sentiment) gets 63/432 of the height
and a 7-metric kernel (morphology) gets 7/432, instead of every kernel
fighting for the same cramped space regardless of how much it actually
produces. This is what makes the view use the whole canvas rather than
clumping everything into one dense mass.
*/
type Point = { id: string; x: number; y: number };

const COLUMN_KERNEL = 0.08;
const COLUMN_METRIC = 0.46;
const COLUMN_CONSUMER = 0.9;

const buildLayout = (
	producers: ProducerRow[],
	consumers: ConsumerRow[],
	width: number,
	height: number,
) => {
	const padding = 28;
	const usableHeight = Math.max(1, height - padding * 2);

	const bySource = new Map<string, ProducerRow[]>();
	for (const p of producers) {
		const list = bySource.get(p.source) ?? [];
		list.push(p);
		bySource.set(p.source, list);
	}

	const sources = [...bySource.keys()].sort();
	const total = producers.length || 1;

	const kernelPoints: Array<Point & { count: number; dead: number }> = [];
	const metricPoints: Point[] = [];
	const metricById = new Map<string, ProducerRow>();

	let cursor = 0;
	for (const source of sources) {
		const rows = (bySource.get(source) ?? []).sort((a, b) => a.metric.localeCompare(b.metric));
		const bandHeight = (rows.length / total) * usableHeight;
		const bandTop = padding + (cursor / total) * usableHeight;

		kernelPoints.push({
			id: source,
			x: width * COLUMN_KERNEL,
			y: bandTop + bandHeight / 2,
			count: rows.length,
			dead: rows.filter((r) => r.dead).length,
		});

		rows.forEach((row, index) => {
			const y =
				rows.length === 1
					? bandTop + bandHeight / 2
					: bandTop + (index / (rows.length - 1)) * Math.max(bandHeight, 1);
			const point = { id: row.id, x: width * COLUMN_METRIC, y };
			metricPoints.push(point);
			metricById.set(row.id, row);
		});

		cursor += rows.length;
	}

	const consumerNames = consumers.map((c) => c.consumer);
	const consumerPoints: Point[] = consumerNames.map((name, index) => ({
		id: name,
		x: width * COLUMN_CONSUMER,
		y:
			consumerNames.length === 1
				? padding + usableHeight / 2
				: padding + (index / (consumerNames.length - 1)) * usableHeight,
	}));

	return { kernelPoints, metricPoints, consumerPoints, metricById };
};

export const MetricLineage = () => {
	const [, setVersion] = useState(0);
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
	const [sourceFilter, setSourceFilter] = useState<string | null>(null);
	const [activeMetricId, setActiveMetricId] = useState<string | null>(null);

	const [dimensions, setDimensions] = useState({ width: 1200, height: 700 });
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		ensureLineageLoaded();
		const unsubscribe = subscribeLineage(() => setVersion((v) => v + 1));
		return unsubscribe;
	}, []);

	const { report, error: loadError } = readLineage();

	useEffect(() => {
		if (!containerRef.current) return;
		const observer = new ResizeObserver((entries) => {
			if (!entries || entries.length === 0) return;
			const { width, height } = entries[0].contentRect;
			setDimensions({ width, height });
		});
		observer.observe(containerRef.current);
		return () => observer.disconnect();
	}, []);

	const sources = useMemo(
		() => (report ? [...new Set(report.producers.map((p) => p.source))].sort() : []),
		[report],
	);

	const filteredProducers = useMemo(() => {
		if (!report) return [];
		return report.producers.filter((p) => {
			if (sourceFilter && p.source !== sourceFilter) return false;
			if (statusFilter === "all") return true;
			return statusOf(p) === statusFilter;
		});
	}, [report, statusFilter, sourceFilter]);

	const relevantConsumers = useMemo(() => {
		if (!report) return [];
		const ids = new Set(filteredProducers.map((p) => p.id));
		return report.consumers.filter((c) =>
			c.targets.some((t) => t === "*" || ids.has(t)),
		);
	}, [report, filteredProducers]);

	const layout = useMemo(
		() => buildLayout(filteredProducers, relevantConsumers, dimensions.width, dimensions.height),
		[filteredProducers, relevantConsumers, dimensions.width, dimensions.height],
	);

	const activeProducer = useMemo(
		() => filteredProducers.find((p) => p.id === activeMetricId) ?? null,
		[filteredProducers, activeMetricId],
	);

	const kernelPointById = useMemo(
		() => new Map(layout.kernelPoints.map((k) => [k.id, k])),
		[layout.kernelPoints],
	);
	const consumerPointById = useMemo(
		() => new Map(layout.consumerPoints.map((c) => [c.id, c])),
		[layout.consumerPoints],
	);

	if (loadError) {
		return (
			<div className="flex h-full w-full flex-col overflow-hidden bg-(--sunken) text-(--f1)">
				<Toolbar>
					<Icon name="target" size="m" className="text-(--f3)" />
					<Typography.Label size="m" tone="f3">
						Metric lineage
					</Typography.Label>
				</Toolbar>
				<div className="flex flex-1 items-center justify-center px-8 text-center font-mono text-[12px] text-(--f4)">
					Could not load /metric-lineage.json ({loadError}). Regenerate it with{" "}
					<span className="text-(--f2)">
						go run ./tools/metriclineage . frontend/public/metric-lineage.json
					</span>
					.
				</div>
			</div>
		);
	}

	if (!report) {
		return (
			<div className="flex h-full w-full flex-col overflow-hidden bg-(--sunken) text-(--f1)">
				<Toolbar>
					<Icon name="target" size="m" className="text-(--f3)" />
					<Typography.Label size="m" tone="f3">
						Metric lineage
					</Typography.Label>
				</Toolbar>
				<div className="flex flex-1 items-center justify-center font-mono text-[12px] text-(--f4)">
					Loading static lineage report…
				</div>
			</div>
		);
	}

	const { summary } = report;

	return (
		<div className="flex h-full w-full flex-col overflow-hidden bg-(--sunken) text-(--f1)">
			<Toolbar>
				<Icon name="target" size="m" className="text-(--f3)" />
				<Typography.Label size="m" tone="f3" className="mr-1 shrink-0">
					Metric lineage
				</Typography.Label>
				<Chip label="metrics" value={summary.totalProducers} />
				<Chip label="unreferenced" value={summary.deadProducers} />
				<Chip label="referenced" value={summary.referencedProducers} />
				<Chip label="kernel-only" value={summary.kernelOnlyProducers} />
				<Chip label="bound refs" value={summary.boundConsumerEdges} />
				<Chip label="catalog refs" value={summary.catalogConsumerEdges} />

				<div className="ml-4 flex items-center gap-1">
					{(["all", "referenced", "kernelOnly", "unreferenced"] as StatusFilter[]).map((filter) => (
						<button
							key={filter}
							type="button"
							onClick={() => setStatusFilter(filter)}
							className={cn(
								"rounded-[3px] border border-(--line) px-2 py-1 font-mono text-[10px] uppercase tracking-wide",
								statusFilter === filter
									? "bg-(--raised) text-(--f1)"
									: "text-(--f4) hover:text-(--f2)",
							)}
						>
							{filter === "kernelOnly" ? "kernel-only" : filter}
						</button>
					))}
				</div>

				<select
					value={sourceFilter ?? ""}
					onChange={(e) => setSourceFilter(e.target.value || null)}
					className="ml-2 rounded-[3px] border border-(--line) bg-(--raised) px-2 py-1 font-mono text-[10px] text-(--f3)"
				>
					<option value="">all kernels</option>
					{sources.map((s) => (
						<option key={s} value={s}>
							{s}
						</option>
					))}
				</select>
			</Toolbar>

			<div className="relative min-h-0 flex-1" ref={containerRef}>
				<svg
					width={dimensions.width}
					height={dimensions.height}
					viewBox={`0 0 ${dimensions.width} ${dimensions.height}`}
					className="absolute inset-0"
				>
					<title>Metric producer to consumer lineage</title>

					{/* Column headers */}
					<text x={dimensions.width * COLUMN_KERNEL} y={16} textAnchor="middle" className="fill-(--f4) font-mono text-[9px] uppercase tracking-wide">
						kernels
					</text>
					<text x={dimensions.width * COLUMN_METRIC} y={16} textAnchor="middle" className="fill-(--f4) font-mono text-[9px] uppercase tracking-wide">
						metrics
					</text>
					<text x={dimensions.width * COLUMN_CONSUMER} y={16} textAnchor="middle" className="fill-(--f4) font-mono text-[9px] uppercase tracking-wide">
						consumers
					</text>

					{/* Kernel -> metric edges (every metric to its own kernel dot) */}
					<g opacity={0.25} stroke="var(--line)" strokeWidth={1}>
						{layout.metricPoints.map((m) => {
							const row = layout.metricById.get(m.id);
							if (!row) return null;
							const k = kernelPointById.get(row.source);
							if (!k) return null;
							return <line key={`k-${m.id}`} x1={k.x} y1={k.y} x2={m.x} y2={m.y} />;
						})}
					</g>

					{/* Metric -> consumer edges, only for fine (named) consumption —
					    kernel/generic edges are structurally many-to-many (every metric
					    of a kernel to one wildcard consumer) and would render as an
					    unreadable solid block; they're shown in the detail panel and
					    consumer list instead. */}
					<g opacity={0.5}>
						{layout.metricPoints.map((m) => {
							const row = layout.metricById.get(m.id);
							if (!row) return null;
							return row.consumers
								.filter((c) => c.kind === "bound" || c.kind === "catalog")
								.map((c) => {
									const target = consumerPointById.get(c.consumer);
									if (!target) return null;
									return (
										<line
											key={`c-${m.id}-${c.consumer}-${c.file}:${c.line}`}
											x1={m.x}
											y1={m.y}
											x2={target.x}
											y2={target.y}
											stroke={STATUS_COLOR.referenced}
											strokeWidth={activeMetricId === m.id ? 2 : 1}
										/>
									);
								});
						})}
					</g>

					{/* Kernel nodes */}
					{layout.kernelPoints.map((k) => (
						<g key={k.id} transform={`translate(${k.x},${k.y})`}>
							<circle
								r={Math.max(4, Math.min(14, 4 + k.count * 0.15))}
								fill="var(--raised)"
								stroke="var(--line)"
								strokeWidth={1.5}
							/>
							<text
								x={14}
								y={4}
								textAnchor="start"
								className="fill-(--f2) font-mono text-[10px] font-semibold"
							>
								{k.id}
							</text>
							<text
								x={14}
								y={16}
								textAnchor="start"
								className="fill-(--f4) font-mono text-[8.5px]"
							>
								{k.count - k.dead}/{k.count} used
							</text>
						</g>
					))}

					{/* Metric nodes */}
					{layout.metricPoints.map((m) => {
						const row = layout.metricById.get(m.id);
						if (!row) return null;
						const status = statusOf(row);
						return (
							// biome-ignore lint/a11y/useSemanticElements: an SVG <g> holding <circle> children can't become a <button>.
							<g
								key={m.id}
								transform={`translate(${m.x},${m.y})`}
								className="cursor-pointer"
								role="button"
								tabIndex={0}
								aria-label={`Select ${row.id}`}
								aria-pressed={activeMetricId === row.id}
								onClick={() => setActiveMetricId(row.id === activeMetricId ? null : row.id)}
								onKeyDown={(event) => {
									if (event.key === "Enter" || event.key === " ") {
										event.preventDefault();
										setActiveMetricId(row.id === activeMetricId ? null : row.id);
									}
								}}
							>
								<circle
									r={activeMetricId === row.id ? 5.5 : 3.5}
									fill={STATUS_COLOR[status]}
									opacity={activeMetricId === row.id ? 1 : 0.85}
									stroke={activeMetricId === row.id ? "var(--f1)" : "none"}
									strokeWidth={1.5}
								/>
								<title>
									{row.id} — {status}
								</title>
							</g>
						);
					})}

					{/* Consumer nodes — dot only; the label renders as an HTML overlay
					    below so long consumer names (e.g. "graph.Solver
					    (causal-influence catalog)") can wrap instead of being
					    clipped at the SVG's right edge. */}
					{layout.consumerPoints.map((c) => {
						const row = relevantConsumers.find((r) => r.consumer === c.id);
						return (
							<g key={c.id} transform={`translate(${c.x},${c.y})`}>
								<circle
									r={5}
									fill={
										row?.kind === "bound" || row?.kind === "catalog"
											? "var(--acc, #5b8def)"
											: row?.kind === "kernel"
												? STATUS_COLOR.kernelOnly
												: "var(--f4)"
									}
									stroke="var(--line)"
									strokeWidth={1}
								/>
							</g>
						);
					})}
				</svg>

				{/* Consumer labels as an HTML overlay, not SVG <text>: SVG text
				    doesn't wrap, so a long consumer name (e.g. "graph.Solver
				    (causal-influence catalog)") either overflows the canvas or has
				    to be truncated. A positioned <div> can wrap onto a second line
				    and use the genuinely large amount of free space to the right of
				    the dot column instead. */}
				{layout.consumerPoints.map((c) => {
					const row = relevantConsumers.find((r) => r.consumer === c.id);
					return (
						<div
							key={`label-${c.id}`}
							className="pointer-events-none absolute font-mono text-[10px] text-(--f3) leading-tight break-words"
							style={{
								left: `${c.x + 12}px`,
								// Anchor the label's first line to the dot's y (not
								// vertically centered): a wrapped 2-line label centered
								// on its own height pushes its first line below the dot
								// it belongs to, which reads as misaligned with the next
								// dot down instead.
								top: `${c.y - 6}px`,
								right: 12,
							}}
						>
							{c.id}
							{row ? (
								<span className="ml-1 text-(--f4)">
									· {row.kind}
								</span>
							) : null}
						</div>
					);
				})}

				{activeProducer ? (
					<Panel
						variant="surface"
						size="bare"
						className="absolute right-3 bottom-3 flex max-w-105 flex-col gap-2 p-3 font-mono text-[11px]"
					>
						<Flex.Row className="items-center justify-between gap-2">
							<Typography.Span className="font-semibold text-(--f1)">
								{activeProducer.id}
							</Typography.Span>
							<Typography.Span
								className="rounded-xs border border-(--line) px-1.5 py-px text-[9px] uppercase"
								style={{ color: STATUS_COLOR[statusOf(activeProducer)] }}
							>
								{statusOf(activeProducer)}
							</Typography.Span>
						</Flex.Row>
						<Typography.Span className="text-[9.5px] text-(--f4)">
							declared {activeProducer.file}:{activeProducer.line}
							{activeProducer.unit ? ` · ${activeProducer.unit}` : ""}
						</Typography.Span>
						<div className="border-(--line) border-t pt-1.5">
							{activeProducer.consumers.length === 0 ? (
								<Typography.Span className="text-(--down)">
									No consumer of any kind found — nothing in the codebase looks at
									this metric.
								</Typography.Span>
							) : (
								<div className="flex flex-col gap-1">
									{activeProducer.consumers.map((c) => (
										<Flex.Row
											key={`${c.consumer}-${c.file}:${c.line}`}
											className="items-center justify-between gap-2"
										>
											<Typography.Span
												className={cn(
													c.kind === "bound" || c.kind === "catalog"
														? "text-(--f1)"
														: c.kind === "kernel"
															? "text-(--f3)"
															: "text-(--f4)",
												)}
											>
												{c.consumer}
											</Typography.Span>
											<Typography.Span className="shrink-0 text-[8.5px] text-(--f4)">
												{c.kind} · {c.file}:{c.line}
											</Typography.Span>
										</Flex.Row>
									))}
								</div>
							)}
						</div>
					</Panel>
				) : (
					<Panel
						variant="surface"
						size="bare"
						className="absolute right-3 bottom-3 max-w-[320px] p-3 font-mono text-[10px] text-(--f4)"
					>
						Click a metric dot for its full producer/consumer trace. Green =
						named by an advisor or the causal schema catalog. Amber =
						kernel-only (a bulk subscription reads the whole kernel, but
						nothing names this metric specifically). Red = no reference
						anywhere.
					</Panel>
				)}
			</div>
		</div>
	);
};
