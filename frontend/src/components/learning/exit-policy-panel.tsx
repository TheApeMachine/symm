import { memo } from "react";
import { decimal } from "#/components/book-readout";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import {
	buildStopChecks,
	meterTone,
	stopRule,
} from "#/lib/stop-telemetry";
import { cn } from "#/lib/utils";

/*
ExitPolicyPanel shows the fixed stop rules this lot is under and how the mark
path is running against them. The simplified stop uses a hard 2pp invalidation
cut and a damage-ratcheted trail (base 1.5pp, floor 0.35pp) — not learned
Causal-MCTS boundaries.

  - the two fixed rules plus live damage / allowed giveback;
  - a P&L trace sparkline of the lot's net path;
  - the same proximity meters as the live panel so open and settled lots read
    identically.

Every figure is optional and null-aware: a restored lot with no trace renders a
waiting sparkline rather than a fabricated line.
*/
export const ExitPolicyPanel = ({
	row,
	className,
}: {
	row: Record<string, unknown>;
	className?: string;
}) => {
	const num = (v: unknown): number | null => decimal(v);

	const peak = num(row.peak_pnl_pct);
	const current = num(row.pnl_pct);

	const structural =
		row.structural && typeof row.structural === "object"
			? (row.structural as Record<string, unknown>)
			: null;

	const PNL_TRACE_CAP = 120;
	const trace = Array.isArray(structural?.pnl_trace)
		? (structural.pnl_trace as unknown[])
				.filter(
					(v): v is number => typeof v === "number" && Number.isFinite(v),
				)
				.slice(-PNL_TRACE_CAP)
		: [];
	const state = structural?.state as Record<string, unknown> | null | undefined;
	const stop =
		state && typeof state === "object" ? (state as Record<string, unknown>) : null;
	const sNum = (k: string): number | null => {
		const v = stop?.[k];
		return typeof v === "number" && Number.isFinite(v) ? v : null;
	};

	const peakStop = sNum("peak_pct") ?? sNum("peakPct") ?? peak;
	const currentStop = sNum("current_pct") ?? sNum("currentPct") ?? current;
	const trailRatio = sNum("trailing");
	const damage = sNum("damage");
	const giveback =
		sNum("giveback_pct") ??
		sNum("givebackPct") ??
		(peakStop !== null && currentStop !== null
			? Math.max(0, peakStop - currentStop)
			: null);
	let allowedGiveback =
		sNum("allowed_giveback_pct") ?? sNum("allowedGivebackPct");
	let remainingGiveback =
		sNum("remaining_giveback_pct") ?? sNum("remainingGivebackPct");
	if (
		allowedGiveback === null &&
		giveback !== null &&
		trailRatio !== null &&
		trailRatio > 0
	) {
		allowedGiveback = giveback / trailRatio;
	}
	if (
		remainingGiveback === null &&
		allowedGiveback !== null &&
		giveback !== null
	) {
		remainingGiveback = Math.max(0, allowedGiveback - giveback);
	}

	const checks = buildStopChecks({
		invalidation: sNum("invalidation") ?? undefined,
		trailing: trailRatio ?? undefined,
		damage: damage ?? undefined,
		current: currentStop,
		peak: peakStop,
		giveback: giveback ?? undefined,
		allowedGiveback: allowedGiveback ?? undefined,
		remainingGiveback: remainingGiveback ?? undefined,
	});

	const invalidationRule = stopRule(
		"Invalidation cut",
		"2.0pp",
		"Hard invalidation: exit when adverse drop from the entry mark reaches 2.0 percentage points.",
	);
	const trailRule = stopRule(
		"Trail base",
		allowedGiveback !== null
			? `${allowedGiveback.toFixed(2)}pp live`
			: "1.5pp base",
		allowedGiveback !== null
			? `Live allowed giveback ${allowedGiveback.toFixed(2)}pp (base 1.5pp, ratcheted by peak and damage, floor 0.35pp).`
			: "Trailing take-profit arms once peak clears +0.5% while still green; base giveback 1.5pp, floor 0.35pp.",
	);
	const damageRule = stopRule(
		"Damage",
		damage !== null ? `${Math.round(damage * 100)}%` : "clean",
		damage !== null && damage > 0
			? `Worst drawdown has burned ${Math.round(damage * 100)}% of the 2pp cut — trail tightens with damage.`
			: "No adverse excursion yet; trail uses the full base giveback room.",
	);
	const entryRule = stopRule(
		"Entry mark",
		(() => {
			const entry = sNum("entry_pct") ?? sNum("entryPct");
			return entry !== null ? `${entry.toFixed(2)}%` : "—";
		})(),
		"First mark PnL the stop treats as entry — adverse drop is measured from here.",
	);

	return (
		<Flex.Column
			gap={2}
			className={cn(
				"rounded-xs border border-(--line) bg-(--surface) p-2 font-mono text-[10.5px]",
				className,
			)}
		>
			<div className="text-[10px] font-bold uppercase tracking-wider text-(--f3)">
				Stop rules
			</div>

			<Grid cols={2} gap={2} responsive={false}>
				<BoundaryFact {...invalidationRule} />
				<BoundaryFact {...trailRule} />
				<BoundaryFact {...damageRule} />
				<BoundaryFact {...entryRule} />
			</Grid>

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
					entry={sNum("entry_pct") ?? sNum("entryPct") ?? 0}
					peak={peak}
					cut={-2}
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
	const known = short !== "—" && short !== "clean";
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
					known ? "text-(--f1)" : "text-(--f4)",
				)}
			>
				{short}
			</div>
		</div>
	);
};

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

		const cutLine = cut !== null ? entry + cut : null;
		const all = [
			...plotTrace,
			entry,
			...(peak !== null ? [peak] : []),
			...(cutLine !== null ? [cutLine] : []),
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
				<line
					x1={0}
					x2={width}
					y1={y(0)}
					y2={y(0)}
					stroke="var(--f4)"
					strokeDasharray="2 2"
					strokeWidth="0.5"
					vectorEffect="non-scaling-stroke"
				/>
				{cutLine !== null ? (
					<line
						x1={0}
						x2={width}
						y1={y(cutLine)}
						y2={y(cutLine)}
						stroke="var(--down)"
						strokeDasharray="3 2"
						strokeWidth="0.6"
						vectorEffect="non-scaling-stroke"
					/>
				) : null}
				{peak !== null && peak > 0 ? (
					<line
						x1={0}
						x2={width}
						y1={y(peak)}
						y2={y(peak)}
						stroke="var(--up)"
						strokeDasharray="2 3"
						strokeWidth="0.5"
						vectorEffect="non-scaling-stroke"
						opacity="0.7"
					/>
				) : null}
				<path
					d={line}
					fill="none"
					stroke={tone}
					strokeWidth="1.1"
					vectorEffect="non-scaling-stroke"
				/>
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
