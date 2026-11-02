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
import { normalizeStoploss } from "#/lib/stoploss";
import { cn } from "#/lib/utils";

/*
ExitPolicyPanel shows DynamicStoploss on a lot: state, floor/peak/friction,
and proximity to floor.
*/
export const ExitPolicyPanel = ({
	row,
	className,
}: {
	row: Record<string, unknown>;
	className?: string;
}) => {
	const current = decimal(row.pnl_pct);
	const stoploss = normalizeStoploss(row.stoploss);

	const peakStop = stoploss?.peak_pnl ?? null;
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
