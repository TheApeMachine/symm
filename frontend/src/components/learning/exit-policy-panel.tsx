import { memo } from "react";
import type { DynamicStoploss } from "#/collections/types";
import { decimal } from "#/components/book-readout";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import {
	buildStopChecks,
	heldVerdict,
	meterTone,
	stateTone,
	stopRule,
} from "#/lib/stop-telemetry";
import { cn } from "#/lib/utils";

/*
ExitPolicyPanel shows DynamicStoploss on a lot: state, floor/peak/friction,
proximity to floor, and the net P&L trace when the desk publishes one.
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

	const stoploss =
		row.stoploss && typeof row.stoploss === "object"
			? (row.stoploss as DynamicStoploss)
			: null;

	const PNL_TRACE_CAP = 120;
	const trace = Array.isArray(row.pnl_trace)
		? (row.pnl_trace as unknown[])
				.filter(
					(v): v is number => typeof v === "number" && Number.isFinite(v),
				)
				.slice(-PNL_TRACE_CAP)
		: [];

	const peakStop = stoploss?.peak_pnl ?? peak;
	const currentStop = stoploss?.current_pnl ?? current;
	const floorStop = stoploss?.has_floor ? stoploss.floor_pnl : null;

	const checks = buildStopChecks({
		state: stoploss?.state,
		proximity: stoploss?.proximity,
		downside_pressure: stoploss?.downside_pressure,
		downside_magnitude_ratio: stoploss?.downside_magnitude_ratio,
		consecutive_down: stoploss?.consecutive_down,
		has_floor: stoploss?.has_floor,
		room_pct: stoploss?.room_pct,
		current: currentStop,
	});

	const verdict = heldVerdict({
		state: stoploss?.state,
		current: currentStop,
		peak: peakStop,
		floor: floorStop,
		friction: stoploss?.friction ?? null,
		has_floor: stoploss?.has_floor,
		proximity: stoploss?.proximity,
		downside_pressure: stoploss?.downside_pressure,
		exit_reason: stoploss?.exit_reason ?? null,
		room_pct: stoploss?.room_pct,
	});

	const stateBadgeVariant = (() => {
		const tone = stateTone(stoploss?.state);
		if (tone === "good") return "success" as const;
		if (tone === "warn") return "warning" as const;
		if (tone === "bad") return "error" as const;
		if (tone === "info") return "info" as const;
		return "disabled" as const;
	})();

	const frictionRule = stopRule(
		"Friction",
		stoploss !== null ? `${stoploss.friction.toFixed(2)}pp` : "—",
		"Minimum peak before the dynamic floor can arm.",
	);
	const peakRule = stopRule(
		"Peak",
		peakStop !== null ? `${peakStop.toFixed(2)}%` : "—",
		"Highest fee-net return this lot has marked.",
	);
	const floorRule = stopRule(
		"Floor",
		floorStop !== null ? `${floorStop.toFixed(2)}%` : "not set",
		stoploss?.has_floor
			? `Profit floor — ${stoploss.room_pct.toFixed(2)}pp room on this mark.`
			: "Floor has not armed yet.",
	);

	return (
		<Flex.Column
			gap={2}
			className={cn(
				"rounded-xs border border-(--line) bg-(--surface) p-2 font-mono text-[10.5px]",
				className,
			)}
		>
			<Flex.Row align="center" justify="between" gap={2}>
				<div className="text-[10px] font-bold uppercase tracking-wider text-(--f3)">
					Dynamic stop
				</div>
				{stoploss ? (
					<Badge size="xxs" variant={stateBadgeVariant} label={stoploss.state} />
				) : (
					<Badge size="xxs" variant="disabled" label="no stop" />
				)}
			</Flex.Row>

			<p className="text-[10px] leading-snug text-(--f2)">{verdict.text}</p>

			<Grid cols={2} gap={2} responsive={false}>
				<BoundaryFact {...frictionRule} />
				<BoundaryFact {...peakRule} />
				<BoundaryFact {...floorRule} />
				<BoundaryFact
					{...stopRule(
						"Mark",
						currentStop !== null ? `${currentStop.toFixed(2)}%` : "—",
						"Current fee-net return on the live book.",
					)}
				/>
			</Grid>

			{trace.length > 0 ? (
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
						peak={peakStop}
						floor={floorStop}
						current={currentStop}
					/>
				</Flex.Column>
			) : null}

			<Flex.Row gap={1} wrap="wrap">
				{checks.map((check) => (
					<Meter
						key={check.name}
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
	const known = short !== "—" && short !== "clean" && short !== "not set";
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
		floor,
		current,
	}: {
		trace: number[];
		entry: number;
		peak: number | null;
		floor: number | null;
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
			...(floor !== null ? [floor] : []),
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
				{floor !== null ? (
					<line
						x1={0}
						x2={width}
						y1={y(floor)}
						y2={y(floor)}
						stroke="var(--warn)"
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
