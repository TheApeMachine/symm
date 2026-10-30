import { memo } from "react";
import { decimal } from "#/components/book-readout";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import {
	buildStopChecks,
	meterTone,
	policyBoundary,
} from "#/lib/stop-telemetry";
import { cn } from "#/lib/utils";

/*
ExitPolicyPanel shows the learned exit policy the desk has tuned for this symbol
and how this particular lot is running against it. It answers "what has the
system learned for this pair, and where does this lot sit under it":

  - the two learned boundaries Causal MCTS derived from this symbol's settled
    excursions (invalidation cut, trailing giveback), plus the empirical
    winning horizon in bars and the worst return seen;
  - a P&L trace sparkline of the lot's net path (the episode the policy learns
    from) with entry, peak, and the stop line;
  - the same four proximity meters as the live panel, so open and settled lots
    read identically.

Every figure is optional and null-aware: a restored lot with no trace renders a
waiting sparkline, and a boundary the desk has not learned reads "not learned"
rather than a fabricated zero.
*/
export const ExitPolicyPanel = ({
	row,
	className,
}: {
	row: Record<string, unknown>;
	className?: string;
}) => {
	const num = (v: unknown): number | null => decimal(v);

	const trailing = num(row.exit_pullback_pct);
	const cut = num(row.nursery_cut_pct);
	const maxBars = num(row.max_lifetime_bars);
	const worst = num(row.worst_pct);
	const peak = num(row.peak_pnl_pct);
	const current = num(row.pnl_pct);
	const barsHeld = num(row.bars_held);

	const trace = Array.isArray(row.pnl_trace)
		? row.pnl_trace.filter(
				(v): v is number => typeof v === "number" && Number.isFinite(v),
			)
		: [];

	const state = row.stop_state as Record<string, unknown> | null | undefined;
	const stop =
		state && typeof state === "object" ? (state as Record<string, unknown>) : null;
	const sNum = (k: string): number | null => {
		const v = stop?.[k];
		return typeof v === "number" && Number.isFinite(v) ? v : null;
	};

	const checks = buildStopChecks({
		invalidation: sNum("invalidation") ?? undefined,
		evaporation: sNum("evaporation") ?? undefined,
		trailing: sNum("trailing") ?? undefined,
		expiry: sNum("expiry") ?? undefined,
		current: sNum("current_pct") ?? current,
		peak: sNum("peak_pct") ?? peak,
		barsHeld: sNum("bars_held") ?? barsHeld ?? undefined,
		maxBars: sNum("maxBars") ?? maxBars ?? undefined,
	});

	const anyBoundary =
		(trailing ?? 0) > 0 || (cut ?? 0) > 0 || (maxBars ?? 0) > 0 || worst !== null;

	return (
		<Flex.Column
			gap={2}
			className={cn(
				"rounded-xs border border-(--line) bg-(--surface) p-2 font-mono text-[10.5px]",
				className,
			)}
		>
			<div className="text-[10px] font-bold uppercase tracking-wider text-(--f3)">
				Learned exit policy
			</div>

			{anyBoundary ? (
				<Grid cols={2} gap={2} responsive={false}>
					<BoundaryFact
						{...policyBoundary("Invalidation cut", cut)}
					/>
					<BoundaryFact {...policyBoundary("Trailing giveback", trailing)} />
					<BoundaryFact
						label="Winning horizon"
						short={maxBars !== null && maxBars > 0 ? `${maxBars} bars` : "not learned"}
						title={`Move-expiry ceiling: the 75th-percentile bar index of winning excursions. ${
							maxBars !== null && maxBars > 0
								? `This lot: ${barsHeld ?? 0} bars held.`
								: "Not learned until this symbol has settled winners."
						}`}
					/>
					<BoundaryFact
						label="Worst seen"
						short={
							worst !== null ? `${worst.toFixed(1)}%` : "not seen"
						}
						title="Worst realized return on this symbol — the floor the Causal MCTS search treats as the cost of a bad cut."
					/>
				</Grid>
			) : (
				<span className="text-(--f4) py-0.5">
					No learned boundaries yet — Causal MCTS tunes the cut, trail, and
					horizon as this symbol settles excursions.
				</span>
			)}

			{/* P&L trajectory: the episode the exit policy learns from. */}
			<Flex.Column gap={1}>
				<Flex.Row justify="between" className="text-[9px] text-(--f4)">
					<span>Net P&L path (one mark per reprice)</span>
					<span className="text-(--f3)">
						{trace.length > 1
							? `${trace.length} marks`
							: peak !== null || current !== null
								? "marking"
								: "no trace"}
					</span>
				</Flex.Row>
				<PnlTrace
					trace={trace}
					entry={0}
					peak={peak}
					cut={cut}
					current={current}
				/>
			</Flex.Column>

			<Flex.Row gap={1} wrap="wrap">
				{checks.map((check) => (
					<Meter
						key={check.id}
						size="xs"
						layout="inline"
						variant={meterTone(check.tone)}
						label={check.name}
						value={check.word.short}
						percent={check.percent ?? 0}
						title={check.word.title}
						labelClassName="text-[8.5px] text-(--f4)"
						valueClassName="text-[8.5px] text-(--f1)"
					/>
				))}
			</Flex.Row>
		</Flex.Column>
	);
};

const BoundaryFact = ({
	label,
	short,
	title,
}: {
	label: string;
	short: string;
	title: string;
}) => {
	const learned = short !== "not learned" && short !== "not seen";
	return (
		<div
			className="rounded-xs border border-(--line) bg-(--sunken) px-2 py-1"
			title={title}
		>
			<div className="text-[8.5px] uppercase tracking-wider text-(--f4)">
				{label}
			</div>
			<div
				className={cn(
					"text-[11px] font-bold tabular-nums",
					learned ? "text-(--f1)" : "text-(--f4)",
				)}
			>
				{short}
			</div>
		</div>
	);
};

/* A compact P&L sparkline over the lot's net path: the trace line, the entry
   baseline, the peak, and the learned cut. A trace shorter than two marks
   renders a waiting strip instead of a fake line. */
const PnlTrace = memo(
	({
		trace,
		entry,
		peak,
		cut,
		current,
	}: {
		trace: number[];
		entry: number;
		peak: number | null;
		cut: number | null;
		current: number | null;
	}) => {
		const width = 100;
		const height = 32;
		const pad = 3;

		const plotTrace =
			trace.length >= 2
				? trace
				: trace.length === 1
					? [trace[0], trace[0]]
					: [entry, entry];

		const all = [
			...plotTrace,
			entry,
			...(peak !== null ? [peak] : []),
			...(cut !== null ? [cut] : []),
			0,
		];
		const min = Math.min(...all);
		const max = Math.max(...all);
		const span = max - min || 1;
		const x = (i: number) =>
			pad + (i / (plotTrace.length - 1)) * (width - pad * 2);
		const y = (v: number) =>
			height - pad - ((v - min) / span) * (height - pad * 2);

		const line = plotTrace
			.map((v, i) => `${i === 0 ? "M" : "L"}${x(i).toFixed(1)},${y(v).toFixed(1)}`)
			.join(" ");
		const last = plotTrace[plotTrace.length - 1];
		const tone =
			current !== null
				? current > 0
					? "var(--up)"
					: current < 0
						? "var(--down)"
						: "var(--f3)"
				: "var(--f3)";

		return (
			<svg
				viewBox={`0 0 ${width} ${height}`}
				preserveAspectRatio="none"
				className="h-8 w-full"
				role="img"
				aria-label="Net P&L trajectory of the lot"
			>
				<title>
					Net P&L trajectory{peak !== null ? ` — peak ${peak.toFixed(1)}%` : ""}
				</title>
				<line x1={0} x2={width} y1={y(0)} y2={y(0)} stroke="var(--f4)" strokeDasharray="2 2" strokeWidth="0.5" vectorEffect="non-scaling-stroke" />
				{cut !== null ? (
					<line x1={0} x2={width} y1={y(cut)} y2={y(cut)} stroke="var(--down)" strokeDasharray="3 2" strokeWidth="0.6" vectorEffect="non-scaling-stroke" />
				) : null}
				{peak !== null && peak > 0 ? (
					<line x1={0} x2={width} y1={y(peak)} y2={y(peak)} stroke="var(--up)" strokeDasharray="2 3" strokeWidth="0.5" vectorEffect="non-scaling-stroke" opacity="0.7" />
				) : null}
				<path d={line} fill="none" stroke={tone} strokeWidth="1.1" vectorEffect="non-scaling-stroke" />
				<circle
					cx={x(plotTrace.length - 1)}
					cy={y(last)}
					r="1.4"
					fill={tone}
				/>
			</svg>
		);
	},
);
PnlTrace.displayName = "PnlTrace";
