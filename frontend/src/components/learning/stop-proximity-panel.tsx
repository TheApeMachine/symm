import { Meter } from "#/components/ui";
import { Flex } from "#/components/ui/flex";
import {
	buildStopChecks,
	heldVerdict,
	meterTone,
	type Tone,
} from "#/lib/stop-telemetry";
import { cn } from "#/lib/utils";

/*
StopProximityPanel renders the simplified stop-loss checks the desk refreshes
on every mark — hard invalidation (2pp), ratcheted trailing take-profit, and
damage — as meters plus one plain-language verdict. This is the "why is this
lot still held, or about to exit, right now" readout.

Data is read directly off the raw desk row (structural.state, peak_pnl_pct).
Everything is optional: a lot that has no stop telemetry yet renders in its
waiting state rather than a fabricated zero.
*/
export type StopRow = Record<string, unknown>;

const num = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const readStop = (row: StopRow) => {
	const structural =
		row.structural && typeof row.structural === "object"
			? (row.structural as Record<string, unknown>)
			: null;
	const state = structural?.state as Record<string, unknown> | null | undefined;
	const stop =
		state && typeof state === "object"
			? (state as Record<string, unknown>)
			: null;

	const current =
		num(stop?.current_pct) ??
		num(stop?.currentPct) ??
		num(row.pnl_pct) ??
		null;
	const peak =
		num(stop?.peak_pct) ?? num(stop?.peakPct) ?? num(row.peak_pnl_pct) ?? null;
	const trailing = num(stop?.trailing);

	const giveback =
		num(stop?.giveback_pct) ??
		num(stop?.givebackPct) ??
		(current !== null && peak !== null ? Math.max(0, peak - current) : null);
	let allowedGiveback =
		num(stop?.allowed_giveback_pct) ?? num(stop?.allowedGivebackPct) ?? null;
	let remainingGiveback =
		num(stop?.remaining_giveback_pct) ??
		num(stop?.remainingGivebackPct) ??
		null;

	if (
		allowedGiveback === null &&
		giveback !== null &&
		trailing !== null &&
		trailing > 0
	) {
		allowedGiveback = giveback / trailing;
	}
	if (
		remainingGiveback === null &&
		allowedGiveback !== null &&
		giveback !== null
	) {
		remainingGiveback = Math.max(0, allowedGiveback - giveback);
	}

	return {
		invalidation: num(stop?.invalidation) ?? undefined,
		trailing: trailing ?? undefined,
		damage: num(stop?.damage) ?? undefined,
		current,
		peak,
		giveback: giveback ?? undefined,
		allowedGiveback: allowedGiveback ?? undefined,
		remainingGiveback: remainingGiveback ?? undefined,
	};
};

export const StopProximityPanel = ({
	row,
	className,
}: {
	row: StopRow;
	className?: string;
}) => {
	const stop = readStop(row);
	const checks = buildStopChecks(stop);
	const verdict = heldVerdict({
		current: stop.current,
		peak: stop.peak,
		invalidation: stop.invalidation ?? null,
		trailing: stop.trailing ?? null,
		damage: stop.damage ?? null,
		giveback: stop.giveback ?? null,
		allowedGiveback: stop.allowedGiveback ?? null,
		remainingGiveback: stop.remainingGiveback ?? null,
	});
	return (
		<Flex.Column
			gap={1}
			className={cn(
				"rounded-xs border border-(--line) bg-(--surface) p-2 font-mono text-[10.5px]",
				className,
			)}
		>
			<Flex.Row align="center" justify="between" className="text-[10px]">
				<span className="font-bold uppercase tracking-wider text-(--f3)">
					Stop proximity
				</span>
				<span
					className="font-bold"
					style={{ color: `var(--${verdictToneColor(verdict.tone)})` }}
					title={verdict.text}
				>
					{verdict.text}
				</span>
			</Flex.Row>

			<Flex.Column gap={1}>
				{checks.map((check) => (
					<Meter
						key={check.id}
						size="s"
						layout="inline"
						variant={meterTone(check.tone)}
						label={check.name}
						value={check.word.short}
						percent={check.percent ?? 0}
						animated
						title={check.word.title}
						labelClassName="text-(--f3)"
						valueClassName="text-(--f1)"
					/>
				))}
			</Flex.Column>
		</Flex.Column>
	);
};

const verdictToneColor = (tone: Tone): string =>
	tone === "good"
		? "up"
		: tone === "warn"
			? "warn"
			: tone === "bad"
				? "down"
				: tone === "info"
					? "info"
					: "f3";
