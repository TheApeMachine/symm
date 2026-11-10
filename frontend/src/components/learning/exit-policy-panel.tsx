import {
	type Position,
	STOPLOSS_ARMED,
	STOPLOSS_HEALTHY,
	STOPLOSS_TRIGGERED,
	type Stoploss,
} from "#/collections/types";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import {
	buildStopChecks,
	heldVerdict,
	meterTone,
	stateTone,
	stopArmed,
	stopRoom,
} from "#/lib/stop-telemetry";
import { cn } from "#/lib/utils";

const stateLabel = (stop: Stoploss) =>
	stop.State === STOPLOSS_TRIGGERED
		? "Triggered"
		: stop.State === STOPLOSS_ARMED
			? "Armed"
			: stop.State === STOPLOSS_HEALTHY
				? "Healthy"
				: "—";

/*
ExitPolicyPanel reads position.stoploss as broker.Stoploss on the wire.
*/
export const ExitPolicyPanel = ({
	row,
	className,
}: {
	row: Position | { stoploss?: Stoploss; pnl_pct?: string | number };
	className?: string;
}) => {
	const stop = row.stoploss;
	const checks = buildStopChecks(stop);
	const verdict = heldVerdict(stop);

	const stateBadgeVariant = (() => {
		const tone = stateTone(stop?.State);
		if (tone === "good") return "success" as const;
		if (tone === "warn") return "warning" as const;
		if (tone === "bad") return "error" as const;
		if (tone === "info") return "info" as const;
		return "disabled" as const;
	})();

	const armed = stop ? stopArmed(stop) : false;

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
				{stop ? (
					<Badge
						size="xxs"
						variant={stateBadgeVariant}
						label={stateLabel(stop)}
					/>
				) : (
					<Badge size="xxs" variant="disabled" label="no stop" />
				)}
			</Flex.Row>

			<p className="text-[10px] leading-snug text-(--f2)">{verdict.text}</p>

			<Grid cols={2} gap={2} responsive={false}>
				<BoundaryFact
					label="Friction"
					short={stop ? `${Math.abs(stop.InitialPnLPct).toFixed(2)}pp` : "—"}
					title="Initial PnL at open."
				/>
				<BoundaryFact
					label="Peak"
					short={stop ? `${stop.PeakPnLPct.toFixed(2)}%` : "—"}
					title="Highest fee-net return this lot has marked."
				/>
				<BoundaryFact
					label="Floor"
					short={armed && stop ? `${stop.FloorPnLPct.toFixed(2)}%` : "not set"}
					title={
						armed && stop
							? `Profit floor — ${stopRoom(stop).toFixed(2)}pp room on this mark.`
							: "Floor has not armed yet."
					}
				/>
				<BoundaryFact
					label="Mark"
					short={stop ? `${stop.CurrentPnLPct.toFixed(2)}%` : "—"}
					title="Current fee-net return on the live book."
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
