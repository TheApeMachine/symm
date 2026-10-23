import { useMemo } from "react";
import { returnTone, signedPct, toneColor } from "#/collections/longtail";
import { useParallelReport } from "#/collections/parallel";
import {
	type ChartSeries,
	MetricChart,
} from "#/components/longtail/metric-chart";
import { Badge, Chip } from "#/components/ui";
import { Flex } from "#/components/ui/flex";

export const ParallelDashboard = () => {
	const { samples } = useParallelReport();

	const latest = samples[samples.length - 1];
	const times = useMemo(() => samples.map((s) => s.at), [samples]);

	const edgeSeries = (book: "A" | "B" | "C"): ChartSeries[] => [
		{
			values: samples.map((s) => s.books[book].meanReturn),
			stroke: toneColor(returnTone(latest?.books[book].meanReturn ?? null)),
			fill: "color-mix(in srgb, var(--f3) 10%, transparent)",
		},
	];

	const incumbentSeries: ChartSeries[] = [
		{
			values: samples.map((s) => s.incumbent?.meanReturn ?? null),
			stroke: "var(--info)",
			fill: "color-mix(in srgb, var(--info) 8%, transparent)",
		},
	];

	return (
		<Flex.Column className="h-full min-h-0 overflow-hidden bg-(--bg) font-mono text-(--f2)">
			<Flex.Row
				align="center"
				justify="between"
				className="shrink-0 border-b border-(--line) bg-(--surface) px-3 py-2"
			>
				<span className="text-[11px] font-bold uppercase tracking-wide text-(--f1)">
					Parallel experiment (A / B / C)
				</span>
				<Flex.Row gap={1}>
					<Chip
						variant="quiet"
						size="s"
						label="samples"
						value={samples.length}
					/>
					<Chip
						variant="quiet"
						size="s"
						label="B − A"
						value={
							latest?.deltaBA === null || latest?.deltaBA === undefined
								? "—"
								: signedPct(latest.deltaBA)
						}
					/>
				</Flex.Row>
			</Flex.Row>

			<div className="grid shrink-0 grid-cols-4 gap-0 border-b border-(--line) bg-(--surface)">
				{(["A", "B", "C"] as const).map((id) => {
					const stats = latest?.books[id];
					const tone = returnTone(stats?.meanReturn ?? null);
					return (
						<div
							key={id}
							className="border-r border-(--line) px-3 py-2 last:border-r-0"
						>
							<div className="text-[9px] uppercase tracking-widest text-(--f4)">
								Book {id}
								{id === "A" && " · control"}
								{id === "B" && " · +physics"}
								{id === "C" && " · physics only"}
							</div>
							<div
								className="text-lg font-bold tabular-nums"
								style={{ color: toneColor(tone) }}
							>
								{stats?.meanReturn === null || stats?.meanReturn === undefined
									? "—"
									: signedPct(stats.meanReturn)}
							</div>
							<div className="text-[10px] text-(--f3)">
								{stats?.closed ?? 0} closed · win{" "}
								{stats?.winRate === null || stats?.winRate === undefined
									? "—"
									: `${Math.round(100 * stats.winRate)}%`}
							</div>
						</div>
					);
				})}
				<div className="px-3 py-2">
					<div className="text-[9px] uppercase tracking-widest text-(--f4)">
						Incumbent strategy
					</div>
					<div
						className="text-lg font-bold tabular-nums"
						style={{
							color: toneColor(
								returnTone(latest?.incumbent?.meanReturn ?? null),
							),
						}}
					>
						{latest?.incumbent?.meanReturn === null ||
						latest?.incumbent?.meanReturn === undefined
							? "—"
							: signedPct(latest.incumbent.meanReturn)}
					</div>
					<div className="text-[10px] text-(--f3)">
						from Longtail virtual book (read-only)
					</div>
				</div>
			</div>

			<div className="grid min-h-0 flex-1 grid-cols-2 grid-rows-2">
				{(
					[
						["A", "Book A — coder only"],
						["B", "Book B — coder + physics gates"],
						["C", "Book C — physics only"],
					] as const
				).map(([id, label]) => (
					<div
						key={id}
						className="flex min-h-0 flex-col border-b border-r border-(--line)"
					>
						<div className="shrink-0 border-b border-(--line) px-2 py-1 text-[10px] font-bold">
							{label}
						</div>
						<MetricChart
							className="flex-1"
							times={times}
							series={edgeSeries(id)}
							formatY={(v) => `${(100 * v).toFixed(1)}%`}
							baseline={0}
						/>
					</div>
				))}
				<div className="flex min-h-0 flex-col border-b border-r border-(--line)">
					<div className="shrink-0 border-b border-(--line) px-2 py-1 text-[10px] font-bold">
						Incumbent vs Book B
					</div>
					<MetricChart
						className="flex-1"
						times={times}
						series={[
							...incumbentSeries,
							{
								label: "B",
								values: samples.map((s) => s.books.B.meanReturn),
								stroke: toneColor(
									returnTone(latest?.books.B.meanReturn ?? null),
								),
							},
						]}
						formatY={(v) => `${(100 * v).toFixed(1)}%`}
						baseline={0}
					/>
				</div>
			</div>

			<Flex.Row className="shrink-0 gap-2 border-t border-(--line) bg-(--surface) px-3 py-2">
				<Badge
					variant="info"
					size="s"
					label="isolated"
					value="no shared policy or desk with strategy"
				/>
				<Badge
					variant={
						(latest?.deltaBA ?? 0) > 0
							? "success"
							: (latest?.deltaBA ?? 0) < 0
								? "warning"
								: "disabled"
					}
					size="s"
					label="physics marginal"
					value={
						latest?.deltaBA === null || latest?.deltaBA === undefined
							? "warming"
							: signedPct(latest.deltaBA)
					}
				/>
				<Badge
					variant="quiet"
					size="s"
					label="C win rate"
					value={
						latest?.books.C.winRate === null ||
						latest?.books.C.winRate === undefined
							? "—"
							: `${Math.round(100 * latest.books.C.winRate)}%`
					}
				/>
				<Badge
					variant={latest?.physics?.healthy ? "success" : "warning"}
					size="s"
					label="Metal field"
					value={
						latest?.physics?.healthy
							? `R ${(latest.physics.kuramotoR ?? 0).toFixed(2)}`
							: "unhealthy"
					}
				/>
				<Badge
					variant="quiet"
					size="s"
					label="gate pass"
					value={
						latest?.gates?.all === undefined
							? "—"
							: `${Math.round(100 * latest.gates.all)}%`
					}
				/>
			</Flex.Row>
		</Flex.Column>
	);
};
